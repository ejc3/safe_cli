package main

import (
	"strings"
	"testing"
)

// TestCleanHumanJSON: the human render drops presigned-URL noise and unescapes `&` (UX
// audit #90 finding 2), while keeping the real fields.
func TestCleanHumanJSON(t *testing.T) {
	body := []byte(`{
		"name": "Apps \u0026 websites",
		"imagePresignedUrl": "https://s3/icon.png?X-Amz-Signature=` + strings.Repeat("a", 250) + `",
		"engines": [
			{"engine": "Google", "imagePresignedUrl": "x", "enabled": true},
			{"engine": "Bing", "url": "https://cdn/img?X-Amz-Credential=` + strings.Repeat("b", 250) + `"}
		]
	}`)
	var out strings.Builder
	if err := cleanHumanJSON(&out, body); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if strings.Contains(s, "imagePresignedUrl") {
		t.Errorf("imagePresignedUrl noise not stripped:\n%s", s)
	}
	if strings.Contains(s, "X-Amz-Signature") || strings.Contains(s, "X-Amz-Credential") {
		t.Errorf("presigned URL value not omitted:\n%s", s)
	}
	if !strings.Contains(s, "Apps & websites") || strings.Contains(s, `\u0026`) {
		t.Errorf("`&` should be unescaped, got:\n%s", s)
	}
	// real content survives
	for _, want := range []string{`"name"`, "Google", "Bing", `"enabled": true`} {
		if !strings.Contains(s, want) {
			t.Errorf("clean output dropped real field %q:\n%s", want, s)
		}
	}
}

// TestCleanHumanJSONFallsBackOnNonJSON: a non-JSON body is passed through untouched.
func TestCleanHumanJSONFallsBackOnNonJSON(t *testing.T) {
	var out strings.Builder
	if err := cleanHumanJSON(&out, []byte("not json")); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "not json" {
		t.Errorf("non-JSON should pass through, got %q", out.String())
	}
}
