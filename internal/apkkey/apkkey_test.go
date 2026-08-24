package apkkey

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- a minimal synthetic .dex builder (test-only) -----------------------------
//
// It emits just enough of the Dalvik format for this package's reader: header,
// string_ids, type_ids, field_ids, one class_def, its class_data, and the
// static_values array. No checksum/signature/map (the reader does not verify them).

type sfield struct {
	name  string
	isStr bool
	str   string // when isStr
	ival  uint32 // when !isStr && !arr
	arr   bool   // emit an (empty) VALUE_ARRAY constant instead
}

func appendUleb(b []byte, v uint64) []byte {
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			c |= 0x80
		}
		b = append(b, c)
		if v == 0 {
			return b
		}
	}
}

func leBytes(v uint32) []byte {
	if v == 0 {
		return []byte{0}
	}
	var b []byte
	for v > 0 {
		b = append(b, byte(v))
		v >>= 8
	}
	return b
}

func encInt(v uint32) []byte {
	lb := leBytes(v)
	return append([]byte{byte((len(lb)-1)<<5) | 0x04}, lb...) // 0x04 = VALUE_INT
}

func encString(idx uint32) []byte {
	lb := leBytes(idx)
	return append([]byte{byte(len(lb)-1)<<5 | valueString}, lb...)
}

func encArray() []byte { // an empty VALUE_ARRAY: 0x1c then uleb size 0
	return []byte{valueArray, 0x00}
}

// dexOpts injects malformed or edge-case layouts for robustness tests.
type dexOpts struct {
	staticFieldsSize  *uint64  // override the declared static_fields_size
	diffs             []uint64 // override the field_idx_diff sequence
	padStrings        int      // add N filler strings so field-value indices exceed 255
	staticValuesCount *uint64  // override the encoded_array size (e.g. shorter than fields)
	zeroClassData     bool     // write class_data_off = 0 in the class_def
}

// buildDex assembles a single-class dex with the given static fields, in order.
func buildDex(className string, fields []sfield) []byte {
	return buildDexEx(className, fields, dexOpts{})
}

func buildDexEx(className string, fields []sfield, opts dexOpts) []byte {
	var strs []string
	strIndex := map[string]uint32{}
	addStr := func(s string) uint32 {
		if idx, ok := strIndex[s]; ok {
			return idx
		}
		idx := uint32(len(strs))
		strs = append(strs, s)
		strIndex[s] = idx
		return idx
	}
	classDescIdx := addStr(className)
	addStr("Ljava/lang/String;")
	addStr("I")
	for i := 0; i < opts.padStrings; i++ {
		addStr(fmt.Sprintf("__pad_%d__", i)) // filler so field-value string indices exceed 255
	}
	for _, f := range fields {
		addStr(f.name)
		if f.isStr {
			addStr(f.str)
		}
	}

	types := []uint32{classDescIdx, strIndex["Ljava/lang/String;"], strIndex["I"]}

	S, T, F := len(strs), len(types), len(fields)
	stringIDsOff := headerSize
	typeIDsOff := stringIDsOff + 4*S
	fieldIDsOff := typeIDsOff + 4*T
	classDefsOff := fieldIDsOff + 8*F
	dataOff := classDefsOff + 32

	// ---- data section: string_data, class_data, static_values ----
	var data []byte
	off := func() int { return dataOff + len(data) }

	stringDataOff := make([]int, S)
	for i, s := range strs {
		stringDataOff[i] = off()
		data = appendUleb(data, uint64(len(s))) // utf16 length (== len for ASCII)
		data = append(data, []byte(s)...)
		data = append(data, 0x00)
	}

	classDataOff := off()
	sfSize := uint64(F)
	if opts.staticFieldsSize != nil {
		sfSize = *opts.staticFieldsSize
	}
	data = appendUleb(data, sfSize) // static_fields_size
	data = appendUleb(data, 0)      // instance_fields_size
	data = appendUleb(data, 0)      // direct_methods_size
	data = appendUleb(data, 0)      // virtual_methods_size
	for i := range fields {
		diff := uint64(1)
		if i == 0 {
			diff = 0
		}
		if opts.diffs != nil {
			diff = opts.diffs[i]
		}
		data = appendUleb(data, diff) // field_idx_diff
		data = appendUleb(data, 0x19) // public static final
	}

	staticValuesOff := off()
	svCount := uint64(F)
	if opts.staticValuesCount != nil {
		svCount = *opts.staticValuesCount
	}
	data = appendUleb(data, svCount) // encoded_array size
	for _, f := range fields {
		switch {
		case f.arr:
			data = append(data, encArray()...)
		case f.isStr:
			data = append(data, encString(strIndex[f.str])...)
		default:
			data = append(data, encInt(f.ival)...)
		}
	}

	total := dataOff + len(data)
	buf := make([]byte, total)
	copy(buf[dataOff:], data)

	copy(buf, []byte("dex\n035\x00"))
	put := func(o int, v uint32) { binary.LittleEndian.PutUint32(buf[o:], v) }
	put(0x20, uint32(total)) // file_size
	put(0x24, headerSize)    // header_size
	put(0x28, 0x12345678)    // endian_tag
	put(offStringIDsSize, uint32(S))
	put(offStringIDsOff, uint32(stringIDsOff))
	put(offTypeIDsSize, uint32(T))
	put(offTypeIDsOff, uint32(typeIDsOff))
	put(offFieldIDsSize, uint32(F))
	put(offFieldIDsOff, uint32(fieldIDsOff))
	put(offClassDefsSize, 1)
	put(offClassDefsOff, uint32(classDefsOff))
	put(0x68, uint32(len(data))) // data_size
	put(0x6c, uint32(dataOff))   // data_off

	for i := 0; i < S; i++ {
		put(stringIDsOff+4*i, uint32(stringDataOff[i]))
	}
	for i := 0; i < T; i++ {
		put(typeIDsOff+4*i, types[i])
	}
	for i, f := range fields {
		fb := fieldIDsOff + 8*i
		binary.LittleEndian.PutUint16(buf[fb:], 0) // class_idx (type_id 0 = the class)
		typeIdx := uint16(1)                       // String
		if !f.isStr {
			typeIdx = 2 // int
		}
		binary.LittleEndian.PutUint16(buf[fb+2:], typeIdx)
		put(fb+4, strIndex[f.name]) // name_idx
	}
	cb := classDefsOff
	put(cb, 0)             // class_idx -> type_id 0
	put(cb+4, 0x11)        // access_flags
	put(cb+8, 0xffffffff)  // superclass_idx = NO_INDEX
	put(cb+12, 0)          // interfaces_off
	put(cb+16, 0xffffffff) // source_file_idx
	put(cb+20, 0)          // annotations_off
	cdOff := uint32(classDataOff)
	if opts.zeroClassData {
		cdOff = 0
	}
	put(cb+24, cdOff)
	put(cb+28, uint32(staticValuesOff))
	return buf
}

func writeAPK(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "app.apk")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for name, b := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

const goodKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const decoyHex = "ffffffff00000001000000000000000000000000ffffffffffffffffffffffff" // looks like an EC constant

// The field's value must be resolved through the class's static_values binding — not
// by scanning for "any 64-hex string". A decoy 64-hex constant sitting in the same
// dex (as real app dexes have: EC curve params, hashes) must be ignored.
func TestExtractResolvesFieldNotAnyHex(t *testing.T) {
	dex := buildDex(BuildConfigClass, []sfield{
		{name: "VERSION_CODE", ival: 42},
		{name: "DECOY_HASH", isStr: true, str: decoyHex}, // a 64-hex decoy, declared BEFORE the target
		{name: SigningFieldName, isStr: true, str: goodKey},
		{name: "BUILD_TYPE", isStr: true, str: "release"},
	})
	apk := writeAPK(t, map[string][]byte{"classes.dex": dex, "AndroidManifest.xml": []byte("x")})
	got, err := ExtractSigningKey(apk)
	if err != nil {
		t.Fatalf("ExtractSigningKey: %v", err)
	}
	if got != goodKey {
		t.Fatalf("got %q, want the HMAC field value %q (a decoy 64-hex string was present)", got, goodKey)
	}
}

// The target may live in classes<N>.dex, not classes.dex.
func TestExtractScansSecondaryDex(t *testing.T) {
	empty := buildDex("Lother/Thing;", []sfield{{name: "X", ival: 1}})
	target := buildDex(BuildConfigClass, []sfield{{name: SigningFieldName, isStr: true, str: goodKey}})
	apk := writeAPK(t, map[string][]byte{"classes.dex": empty, "classes7.dex": target})
	got, err := ExtractSigningKey(apk)
	if err != nil {
		t.Fatalf("ExtractSigningKey: %v", err)
	}
	if got != goodKey {
		t.Fatalf("got %q, want %q", got, goodKey)
	}
}

// A same-named field on a different class (wrong package) must be ignored; only the
// exact BuildConfig descriptor counts.
func TestExtractIgnoresWrongPackageBuildConfig(t *testing.T) {
	const wrong = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	other := buildDex("Lcom/other/BuildConfig;", []sfield{{name: SigningFieldName, isStr: true, str: wrong}})
	target := buildDex(BuildConfigClass, []sfield{{name: SigningFieldName, isStr: true, str: goodKey}})
	apk := writeAPK(t, map[string][]byte{"classes.dex": other, "classes2.dex": target})
	got, err := ExtractSigningKey(apk)
	if err != nil {
		t.Fatalf("ExtractSigningKey: %v", err)
	}
	if got != goodKey {
		t.Fatalf("got %q, want the verizon BuildConfig value %q, not the wrong-package one", got, goodKey)
	}
}

// The value's string_id index may need more than one byte (VALUE_STRING with value_arg>0).
func TestExtractMultiByteStringIndex(t *testing.T) {
	dex := buildDexEx(BuildConfigClass,
		[]sfield{{name: SigningFieldName, isStr: true, str: goodKey}},
		dexOpts{padStrings: 400}, // pushes goodKey's string index past 255 -> 2-byte index
	)
	got, err := ExtractSigningKey(writeAPK(t, map[string][]byte{"classes.dex": dex}))
	if err != nil {
		t.Fatalf("ExtractSigningKey: %v", err)
	}
	if got != goodKey {
		t.Fatalf("got %q, want %q (multi-byte string index)", got, goodKey)
	}
}

// If the field exists but its constant is not a string, it must not be mis-decoded.
func TestTargetNonStringConstant(t *testing.T) {
	dex := buildDex(BuildConfigClass, []sfield{{name: SigningFieldName, ival: 42}})
	_, err := ExtractSigningKey(writeAPK(t, map[string][]byte{"classes.dex": dex}))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found for a non-string constant, got %v", err)
	}
}

// A VALUE_ARRAY-typed field declared before the target must be skipped correctly by
// the encoded_array reader's recursion.
func TestArrayValuedFieldBeforeTarget(t *testing.T) {
	dex := buildDex(BuildConfigClass, []sfield{
		{name: "SOME_ARRAY", arr: true},
		{name: SigningFieldName, isStr: true, str: goodKey},
	})
	got, err := ExtractSigningKey(writeAPK(t, map[string][]byte{"classes.dex": dex}))
	if err != nil {
		t.Fatalf("ExtractSigningKey: %v", err)
	}
	if got != goodKey {
		t.Fatalf("got %q, want %q (array-valued field before target)", got, goodKey)
	}
}

// When static_values is shorter than static_fields (trailing default values), a target
// beyond the array is treated as absent, not read out of bounds.
func TestTrailingDefaultShorterArray(t *testing.T) {
	two := uint64(2)
	dex := buildDexEx(BuildConfigClass, []sfield{
		{name: "A", ival: 1},
		{name: "B", ival: 2},
		{name: SigningFieldName, isStr: true, str: goodKey}, // position 2, but array declares only 2
	}, dexOpts{staticValuesCount: &two})
	_, err := ExtractSigningKey(writeAPK(t, map[string][]byte{"classes.dex": dex}))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found when the value is a trailing default, got %v", err)
	}
}

// A matching class with class_data_off == 0 carries no static constants.
func TestZeroClassData(t *testing.T) {
	dex := buildDexEx(BuildConfigClass,
		[]sfield{{name: SigningFieldName, isStr: true, str: goodKey}},
		dexOpts{zeroClassData: true},
	)
	_, err := ExtractSigningKey(writeAPK(t, map[string][]byte{"classes.dex": dex}))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found when class_data_off is 0, got %v", err)
	}
}

func TestExtractRejectsNon64Hex(t *testing.T) {
	dex := buildDex(BuildConfigClass, []sfield{{name: SigningFieldName, isStr: true, str: "not-a-hex-key"}})
	apk := writeAPK(t, map[string][]byte{"classes.dex": dex})
	if _, err := ExtractSigningKey(apk); err == nil {
		t.Fatal("want error for a non-64-hex value, got nil")
	}
}

func TestExtractFieldAbsent(t *testing.T) {
	dex := buildDex(BuildConfigClass, []sfield{{name: "SOMETHING_ELSE", isStr: true, str: "v"}})
	apk := writeAPK(t, map[string][]byte{"classes.dex": dex})
	_, err := ExtractSigningKey(apk)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want a not-found error, got %v", err)
	}
}

func TestExtractNoDexIsNotAnAPK(t *testing.T) {
	apk := writeAPK(t, map[string][]byte{"AndroidManifest.xml": []byte("x"), "res/x.png": []byte("y")})
	_, err := ExtractSigningKey(apk)
	if err == nil || !strings.Contains(err.Error(), "no classes*.dex") {
		t.Fatalf("want a no-dex error, got %v", err)
	}
}

// A malformed static_fields list with a non-ascending field_idx (a zero diff after
// the first entry) must be rejected, not walked — that is the shape that would
// otherwise pin the index and re-scan one name every iteration (quadratic blowup).
func TestRejectsNonAscendingStaticFields(t *testing.T) {
	dex := buildDexEx(BuildConfigClass,
		[]sfield{{name: "VERSION_CODE", ival: 1}, {name: SigningFieldName, isStr: true, str: goodKey}},
		dexOpts{diffs: []uint64{0, 0}}, // second field pins field_idx at 0
	)
	_, _, err := findStaticStringField(dex, BuildConfigClass, SigningFieldName)
	if err == nil || !strings.Contains(err.Error(), "ascending") {
		t.Fatalf("want a non-ascending error, got %v", err)
	}
}

// An inflated static_fields_size (more static fields than the dex has field_ids)
// must be rejected up front, bounding the loop.
func TestRejectsInflatedStaticFieldsSize(t *testing.T) {
	huge := uint64(1 << 20)
	dex := buildDexEx(BuildConfigClass,
		[]sfield{{name: SigningFieldName, isStr: true, str: goodKey}},
		dexOpts{staticFieldsSize: &huge},
	)
	_, _, err := findStaticStringField(dex, BuildConfigClass, SigningFieldName)
	if err == nil || !strings.Contains(err.Error(), "exceeds field_ids") {
		t.Fatalf("want an inflated-size error, got %v", err)
	}
}

// The dex parser must never panic on arbitrary input, only return a value or error.
func FuzzFindStaticStringField(f *testing.F) {
	f.Add(buildDex(BuildConfigClass, []sfield{{name: SigningFieldName, isStr: true, str: goodKey}}))
	f.Add(buildDex(BuildConfigClass, []sfield{{name: "A", ival: 1}, {name: SigningFieldName, isStr: true, str: goodKey}}))
	f.Add([]byte("dex\n035\x00"))
	f.Add([]byte{})
	f.Fuzz(func(_ *testing.T, b []byte) {
		// Any (value, found, err) is acceptable; a panic is not.
		_, _, _ = findStaticStringField(b, BuildConfigClass, SigningFieldName)
	})
}

// Malformed/truncated dex bytes must return an error, never panic.
func TestMalformedDexDoesNotPanic(t *testing.T) {
	full := buildDex(BuildConfigClass, []sfield{{name: SigningFieldName, isStr: true, str: goodKey}})
	cases := map[string][]byte{
		"empty":          {},
		"magic-only":     []byte("dex\n035\x00"),
		"bad-magic":      bytes.Repeat([]byte{0xff}, 256),
		"truncated-half": full[:len(full)/2],
		"truncated-head": full[:headerSize-1],
		"garbage-offsets": func() []byte {
			b := append([]byte(nil), full...)
			binary.LittleEndian.PutUint32(b[offStringIDsOff:], 0xffffff00)
			return b
		}(),
	}
	for name, b := range cases {
		apk := writeAPK(t, map[string][]byte{"classes.dex": b})
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s: panicked: %v", name, r)
				}
			}()
			if _, err := ExtractSigningKey(apk); err == nil {
				t.Errorf("%s: want an error, got nil", name)
			}
		}()
	}
}
