package apkkey

import (
	"os"
	"path/filepath"
	"testing"
)

const resolveKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestResolveKeyFromEnv(t *testing.T) {
	t.Setenv(EnvSigningKey, resolveKey)
	t.Setenv(EnvSigningKeyFile, "") // ensure file path is not consulted
	got, err := ResolveKey()
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if got != resolveKey {
		t.Fatalf("got %q, want %q", got, resolveKey)
	}
}

func TestResolveKeyEnvTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "k")
	if err := os.WriteFile(f, []byte("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSigningKey, resolveKey)
	t.Setenv(EnvSigningKeyFile, f)
	got, err := ResolveKey()
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if got != resolveKey {
		t.Fatalf("env should win; got %q", got)
	}
}

func TestResolveKeyFromFileTrimmed(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "k")
	if err := os.WriteFile(f, []byte("  "+resolveKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSigningKey, "")
	t.Setenv(EnvSigningKeyFile, f)
	got, err := ResolveKey()
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if got != resolveKey {
		t.Fatalf("got %q, want the trimmed key", got)
	}
}

func TestResolveKeyRejectsBadEnv(t *testing.T) {
	t.Setenv(EnvSigningKey, "not-hex")
	if _, err := ResolveKey(); err == nil {
		t.Fatal("want error for a non-hex env value")
	}
}

func TestResolveKeyRejectsBadFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "k")
	if err := os.WriteFile(f, []byte("deadbeef"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSigningKey, "")
	t.Setenv(EnvSigningKeyFile, f)
	if _, err := ResolveKey(); err == nil {
		t.Fatal("want error for a short/invalid file value")
	}
}

func TestResolveKeyMissing(t *testing.T) {
	t.Setenv(EnvSigningKey, "")
	t.Setenv(EnvSigningKeyFile, "")
	_, err := ResolveKey()
	if err == nil {
		t.Fatal("want error when neither var is set")
	}
}
