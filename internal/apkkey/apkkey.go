// Package apkkey extracts the app's HMAC request-signing key from a user-supplied
// APK. The key (BuildConfig.HMAC_SIGNING_SECRET) is not shipped with this tool — the
// operator provides their own licensed APK and this package reads the constant out of
// it locally. See docs/PROCESS.md §11 and CLAUDE.md § Secrets.
//
// The reader is a minimal, bounds-checked Dalvik (.dex) parser. It does NOT search the
// string pool for "any 64-hex string" — a real app dex contains unrelated 64-hex
// constants (elliptic-curve parameters, hashes). Instead it resolves the specific
// static field BuildConfig.HMAC_SIGNING_SECRET via the class's static_values array,
// which is the only correct way to bind a field name to its constant value.
//
// The extracted value is returned to the caller and never logged or printed here.
package apkkey

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// Target identifies the field to extract.
const (
	BuildConfigClass = "Lcom/verizon/familybase/feature/identity/BuildConfig;"
	SigningFieldName = "HMAC_SIGNING_SECRET"
)

// maxDexSize bounds a single dex we will read into memory. The app's largest dex is
// ~23 MiB; 96 MiB is comfortable headroom while refusing absurd/hostile inputs.
const maxDexSize = 96 << 20

var (
	errBounds = errors.New("dex: offset out of bounds")
	hex64     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	dexMagic  = []byte("dex\n")
)

// ExtractSigningKey opens an APK (a zip) and returns the signing key read from
// BuildConfig.HMAC_SIGNING_SECRET. It scans each classes*.dex until the field is
// found. The returned value is validated as 64 lowercase hex characters. The value is
// never logged or printed by this package.
func ExtractSigningKey(apkPath string) (string, error) {
	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		return "", fmt.Errorf("open apk %q: %w", apkPath, err)
	}
	defer func() { _ = zr.Close() }()

	var sawDex bool
	var lastErr error
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if !strings.HasPrefix(base, "classes") || !strings.HasSuffix(base, ".dex") {
			continue
		}
		sawDex = true
		if f.UncompressedSize64 > maxDexSize {
			lastErr = fmt.Errorf("%s is %d bytes, over the %d limit", f.Name, f.UncompressedSize64, maxDexSize)
			continue
		}
		dex, err := readZipEntry(f)
		if err != nil {
			lastErr = fmt.Errorf("read %s: %w", f.Name, err)
			continue
		}
		val, found, err := findStaticStringField(dex, BuildConfigClass, SigningFieldName)
		if err != nil {
			lastErr = fmt.Errorf("parse %s: %w", f.Name, err)
			continue
		}
		if found {
			if !hex64.MatchString(val) {
				return "", fmt.Errorf("%s in %s is not 64 lowercase hex characters", SigningFieldName, f.Name)
			}
			return val, nil
		}
	}
	if !sawDex {
		return "", fmt.Errorf("%q contains no classes*.dex (is it an APK?)", apkPath)
	}
	if lastErr != nil {
		return "", fmt.Errorf("%s not found; last error: %w", SigningFieldName, lastErr)
	}
	return "", fmt.Errorf("%s not found in any classes*.dex of %q", SigningFieldName, apkPath)
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	// Read at most maxDexSize+1 so a lying UncompressedSize64 cannot force an
	// unbounded allocation; reject if it exceeds the cap.
	b, err := io.ReadAll(io.LimitReader(rc, maxDexSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxDexSize {
		return nil, errors.New("dex exceeds size limit")
	}
	return b, nil
}

// ---- minimal dex reader -------------------------------------------------------

// Header field offsets (little-endian) per the Dalvik .dex format.
const (
	offStringIDsSize = 0x38
	offStringIDsOff  = 0x3C
	offTypeIDsSize   = 0x40
	offTypeIDsOff    = 0x44
	offFieldIDsSize  = 0x50
	offFieldIDsOff   = 0x54
	offClassDefsSize = 0x60
	offClassDefsOff  = 0x64
	headerSize       = 0x70
)

// encoded_value type tags we care about.
const (
	valueByte     = 0x00
	valueString   = 0x17
	valueArray    = 0x1c
	valueAnnot    = 0x1d
	valueNull     = 0x1e
	valueBoolean  = 0x1f
	valueTypeMask = 0x1f
)

type dex struct {
	b            []byte
	stringIDsOff uint32
	stringIDsLen uint32
	typeIDsOff   uint32
	typeIDsLen   uint32
	fieldIDsOff  uint32
	fieldIDsLen  uint32
	classDefsOff uint32
	classDefsLen uint32
}

func (d *dex) u8(off int) (byte, error) {
	if off < 0 || off >= len(d.b) {
		return 0, errBounds
	}
	return d.b[off], nil
}

func (d *dex) u32(off int) (uint32, error) {
	if off < 0 || off+4 > len(d.b) {
		return 0, errBounds
	}
	return binary.LittleEndian.Uint32(d.b[off:]), nil
}

// uleb128 reads an unsigned LEB128 at off, returning the value and the number of
// bytes consumed. It rejects encodings longer than 5 bytes (u32 range).
func uleb128(b []byte, off int) (val uint64, n int, err error) {
	var shift uint
	for i := 0; i < 5; i++ {
		if off+i < 0 || off+i >= len(b) {
			return 0, 0, errBounds
		}
		c := b[off+i]
		val |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return val, i + 1, nil
		}
		shift += 7
	}
	return 0, 0, errors.New("dex: uleb128 too long")
}

// readUintLE reads a little-endian unsigned integer of size bytes (1..4).
func (d *dex) readUintLE(off, size int) (uint32, error) {
	if size < 1 || size > 4 {
		return 0, fmt.Errorf("dex: bad int size %d", size)
	}
	if off < 0 || off+size > len(d.b) {
		return 0, errBounds
	}
	var v uint32
	for i := 0; i < size; i++ {
		v |= uint32(d.b[off+i]) << (8 * uint(i))
	}
	return v, nil
}

func newDex(b []byte) (*dex, error) {
	if len(b) < headerSize {
		return nil, errors.New("dex: too small for header")
	}
	if !hasPrefix(b, dexMagic) {
		return nil, errors.New("dex: bad magic")
	}
	d := &dex{b: b}
	get := func(off int) (uint32, error) { return d.u32(off) }
	var err error
	if d.stringIDsLen, err = get(offStringIDsSize); err != nil {
		return nil, err
	}
	if d.stringIDsOff, err = get(offStringIDsOff); err != nil {
		return nil, err
	}
	if d.typeIDsLen, err = get(offTypeIDsSize); err != nil {
		return nil, err
	}
	if d.typeIDsOff, err = get(offTypeIDsOff); err != nil {
		return nil, err
	}
	if d.fieldIDsLen, err = get(offFieldIDsSize); err != nil {
		return nil, err
	}
	if d.fieldIDsOff, err = get(offFieldIDsOff); err != nil {
		return nil, err
	}
	if d.classDefsLen, err = get(offClassDefsSize); err != nil {
		return nil, err
	}
	if d.classDefsOff, err = get(offClassDefsOff); err != nil {
		return nil, err
	}
	return d, nil
}

func hasPrefix(b, prefix []byte) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := range prefix {
		if b[i] != prefix[i] {
			return false
		}
	}
	return true
}

// stringAt resolves a string_id index to its decoded value. dex string_data is a
// uleb128 utf16 length followed by MUTF-8 bytes terminated by a 0x00 byte; for the
// ASCII identifiers and hex value we read, the raw bytes are the string.
func (d *dex) stringAt(idx uint32) (string, error) {
	if idx >= d.stringIDsLen {
		return "", errBounds
	}
	dataOff, err := d.u32(int(d.stringIDsOff) + 4*int(idx))
	if err != nil {
		return "", err
	}
	_, n, err := uleb128(d.b, int(dataOff)) // skip the utf16 length prefix
	if err != nil {
		return "", err
	}
	start := int(dataOff) + n
	end := start
	for end < len(d.b) && d.b[end] != 0x00 {
		end++
	}
	if end >= len(d.b) {
		return "", errBounds // unterminated string
	}
	return string(d.b[start:end]), nil
}

// typeDescriptor resolves a type_id index to its descriptor string (e.g. "Lx/Y;").
func (d *dex) typeDescriptor(idx uint32) (string, error) {
	if idx >= d.typeIDsLen {
		return "", errBounds
	}
	strIdx, err := d.u32(int(d.typeIDsOff) + 4*int(idx))
	if err != nil {
		return "", err
	}
	return d.stringAt(strIdx)
}

// fieldName resolves a field_id index to its member name.
func (d *dex) fieldName(idx uint32) (string, error) {
	if idx >= d.fieldIDsLen {
		return "", errBounds
	}
	// field_id_item: class_idx u16, type_idx u16, name_idx u32
	nameIdx, err := d.u32(int(d.fieldIDsOff) + 8*int(idx) + 4)
	if err != nil {
		return "", err
	}
	return d.stringAt(nameIdx)
}

// findStaticStringField returns the string value of the static field classDesc.field,
// bound through the class's static_values array. found is false if the class exists
// but the field is absent or has no constant value; err is for malformed input.
func findStaticStringField(b []byte, classDesc, field string) (val string, found bool, err error) {
	d, err := newDex(b)
	if err != nil {
		return "", false, err
	}
	for i := uint32(0); i < d.classDefsLen; i++ {
		base := int(d.classDefsOff) + 32*int(i)
		classIdx, err := d.u32(base)
		if err != nil {
			return "", false, err
		}
		desc, err := d.typeDescriptor(classIdx)
		if err != nil {
			return "", false, err
		}
		if desc != classDesc {
			continue
		}
		classDataOff, err := d.u32(base + 24)
		if err != nil {
			return "", false, err
		}
		staticValuesOff, err := d.u32(base + 28)
		if err != nil {
			return "", false, err
		}
		if classDataOff == 0 || staticValuesOff == 0 {
			return "", false, nil // class matched but carries no static constant values
		}
		return d.staticStringValue(int(classDataOff), int(staticValuesOff), field)
	}
	return "", false, nil // class not in this dex
}

// staticStringValue walks the class_data static_fields (ascending field_idx) to find
// the position of `field`, then reads the same position from the static_values array.
func (d *dex) staticStringValue(classDataOff, staticValuesOff int, field string) (string, bool, error) {
	off := classDataOff
	staticFieldsSize, n, err := uleb128(d.b, off)
	if err != nil {
		return "", false, err
	}
	off += n
	// skip instance_fields_size, direct_methods_size, virtual_methods_size
	for i := 0; i < 3; i++ {
		if _, n, err = uleb128(d.b, off); err != nil {
			return "", false, err
		}
		off += n
	}
	fieldPos := -1
	var fieldIdx uint64
	for i := uint64(0); i < staticFieldsSize; i++ {
		diff, n, err := uleb128(d.b, off) // field_idx_diff
		if err != nil {
			return "", false, err
		}
		off += n
		if _, n, err = uleb128(d.b, off); err != nil { // access_flags
			return "", false, err
		}
		off += n
		fieldIdx += diff
		name, err := d.fieldName(uint32(fieldIdx))
		if err != nil {
			return "", false, err
		}
		if name == field {
			fieldPos = int(i)
			break
		}
	}
	if fieldPos < 0 {
		return "", false, nil // field not present on this class
	}
	return d.encodedArrayStringAt(staticValuesOff, fieldPos)
}

// encodedArrayStringAt reads the k-th value of the encoded_array at off and, if it is
// a VALUE_STRING, resolves it. A k beyond the array (a trailing default value) means
// the field is not a committed string constant.
func (d *dex) encodedArrayStringAt(off, k int) (string, bool, error) {
	size, n, err := uleb128(d.b, off)
	if err != nil {
		return "", false, err
	}
	off += n
	if uint64(k) >= size {
		return "", false, nil
	}
	for i := 0; i < int(size); i++ {
		strIdx, isString, next, err := d.readEncodedValue(off, 0)
		if err != nil {
			return "", false, err
		}
		if i == k {
			if !isString {
				return "", false, nil // field exists but its constant is not a string
			}
			s, err := d.stringAt(strIdx)
			return s, err == nil, err
		}
		off = next
	}
	return "", false, nil
}

// maxEncodedDepth caps nesting of encoded_array / encoded_annotation values so a
// crafted or corrupt dex cannot drive readEncodedValue into a stack overflow.
const maxEncodedDepth = 64

// readEncodedValue parses one encoded_value at off. It returns the string_id index and
// isString=true for VALUE_STRING, and always returns next = the offset just past the
// value (recursing through ARRAY/ANNOTATION) so callers can skip values they don't want.
func (d *dex) readEncodedValue(off, depth int) (strIdx uint32, isString bool, next int, err error) {
	if depth > maxEncodedDepth {
		return 0, false, 0, errors.New("dex: encoded value nested too deeply")
	}
	hdr, err := d.u8(off)
	if err != nil {
		return 0, false, 0, err
	}
	off++
	vtype := hdr & valueTypeMask
	varg := int(hdr >> 5)
	switch vtype {
	case valueArray:
		size, n, err := uleb128(d.b, off)
		if err != nil {
			return 0, false, 0, err
		}
		off += n
		for i := uint64(0); i < size; i++ {
			if _, _, off, err = d.readEncodedValue(off, depth+1); err != nil {
				return 0, false, 0, err
			}
		}
		return 0, false, off, nil
	case valueAnnot:
		// encoded_annotation: type_idx uleb, size uleb, size*(name_idx uleb + value)
		_, n, err := uleb128(d.b, off) // type_idx
		if err != nil {
			return 0, false, 0, err
		}
		off += n
		size, n2, err := uleb128(d.b, off)
		if err != nil {
			return 0, false, 0, err
		}
		off += n2
		for i := uint64(0); i < size; i++ {
			_, n3, err := uleb128(d.b, off) // name_idx
			if err != nil {
				return 0, false, 0, err
			}
			off += n3
			if _, _, off, err = d.readEncodedValue(off, depth+1); err != nil {
				return 0, false, 0, err
			}
		}
		return 0, false, off, nil
	case valueNull, valueBoolean:
		return 0, false, off, nil // no payload (boolean is encoded in value_arg)
	case valueString:
		size := varg + 1
		v, err := d.readUintLE(off, size)
		if err != nil {
			return 0, false, 0, err
		}
		return v, true, off + size, nil
	default:
		// byte/short/char/int/long/float/double/type/field/method/enum/method_type/
		// method_handle: a (value_arg+1)-byte payload we don't need.
		_ = valueByte
		return 0, false, off + varg + 1, nil
	}
}
