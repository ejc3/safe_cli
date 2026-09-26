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

// TestEmptyRecordBody: a "no records" 404 that carries a real data payload (call/text
// activity with zero totals) is recognized and the error envelope stripped; a pure error
// envelope is NOT (it stays a genuine not-found). UX audit #90 — `calls log` on a quiet week.
func TestEmptyRecordBody(t *testing.T) {
	data := []byte(`{"statusCode":404,"totalCalls":0,"callActivity":null,"textActivity":null,"errors":[{"code":404,"message":"resource not found"}]}`)
	clean, ok := emptyRecordBody(data)
	if !ok {
		t.Fatal("a data-shaped 404 should be recognized as an empty result")
	}
	if strings.Contains(string(clean), "errors") || strings.Contains(string(clean), "statusCode") {
		t.Errorf("error envelope not stripped: %s", clean)
	}
	if !strings.Contains(string(clean), "totalCalls") || !strings.Contains(string(clean), "callActivity") {
		t.Errorf("real data keys dropped: %s", clean)
	}

	// a pure error envelope is a genuine not-found, not an empty result
	if _, ok := emptyRecordBody([]byte(`{"statusCode":404,"errors":[{"code":404,"message":"resource not found"}]}`)); ok {
		t.Error("a pure error envelope must NOT be treated as an empty result")
	}
	if _, ok := emptyRecordBody([]byte("not json")); ok {
		t.Error("non-JSON must not be treated as an empty result")
	}
}
