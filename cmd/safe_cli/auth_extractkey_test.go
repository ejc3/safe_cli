package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func withStubExtractor(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := extractSigningKey
	extractSigningKey = fn
	t.Cleanup(func() { extractSigningKey = orig })
}

func TestAuthExtractKeyPlain(t *testing.T) {
	withStubExtractor(t, func(string) (string, error) { return testKey, nil })
	var out bytes.Buffer
	rc := &runContext{G: &Globals{}, Out: &out}
	if err := (&authExtractKeyCmd{APK: "any.apk"}).Run(rc); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != testKey {
		t.Fatalf("plain output = %q, want the key", got)
	}
}

func TestAuthExtractKeyJSON(t *testing.T) {
	withStubExtractor(t, func(string) (string, error) { return testKey, nil })
	var out bytes.Buffer
	rc := &runContext{G: &Globals{JSON: true}, Out: &out}
	if err := (&authExtractKeyCmd{APK: "any.apk"}).Run(rc); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, out.String())
	}
	if m["signing_key"] != testKey {
		t.Fatalf("json signing_key = %q, want the key", m["signing_key"])
	}
}

func TestAuthExtractKeyPropagatesError(t *testing.T) {
	withStubExtractor(t, func(string) (string, error) { return "", errors.New("no dex") })
	var out bytes.Buffer
	err := (&authExtractKeyCmd{APK: "any.apk"}).Run(&runContext{G: &Globals{}, Out: &out})
	if err == nil {
		t.Fatal("want the extractor error to propagate, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("nothing should be printed on error, got %q", out.String())
	}
}
