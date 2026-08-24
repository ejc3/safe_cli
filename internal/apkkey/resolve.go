package apkkey

import (
	"fmt"
	"os"
	"strings"
)

// Environment variables the operator uses to supply the signing key at runtime.
const (
	EnvSigningKey     = "SAFE_CLI_SIGNING_KEY"      // the 64-hex value directly
	EnvSigningKeyFile = "SAFE_CLI_SIGNING_KEY_FILE" // path to a file holding the value
)

// ResolveKey returns the app signing key from the environment. SAFE_CLI_SIGNING_KEY
// takes precedence; otherwise SAFE_CLI_SIGNING_KEY_FILE names a file whose contents are
// the key. The value is validated as 64 lowercase hex characters. It is never logged.
// Obtain it with `safe_cli auth extract-key --apk <your.apk>`.
func ResolveKey() (string, error) {
	if v := strings.TrimSpace(os.Getenv(EnvSigningKey)); v != "" {
		if !hex64.MatchString(v) {
			return "", fmt.Errorf("%s is not 64 lowercase hex characters", EnvSigningKey)
		}
		return v, nil
	}
	if f := os.Getenv(EnvSigningKeyFile); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", EnvSigningKeyFile, err)
		}
		v := strings.TrimSpace(string(b))
		if !hex64.MatchString(v) {
			return "", fmt.Errorf("%s (%s) does not contain a 64-hex key", EnvSigningKeyFile, f)
		}
		return v, nil
	}
	return "", fmt.Errorf("no signing key: set %s or %s (get it with `safe_cli auth extract-key --apk <your.apk>`)", EnvSigningKey, EnvSigningKeyFile)
}
