// Package deviceid provides a stable per-install app UUID for the signed device-auth
// requests (the x-appuuid header). It is a random UUIDv4 generated on first use and
// persisted under the user's config dir. It is NOT a secret — it identifies this
// install to the backend, like a device id — so persisting it in the clear is fine.
package deviceid

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AppUUID returns this install's app UUID, creating and persisting one on first use.
// The path honors XDG_CONFIG_HOME (via os.UserConfigDir).
func AppUUID() (string, error) {
	path, err := defaultPath()
	if err != nil {
		return "", err
	}
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); uuidRE.MatchString(s) {
			return s, nil
		}
	}
	u, err := newV4()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(u+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return u, nil
}

func defaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "safe_cli", "appuuid"), nil
}

func newV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
