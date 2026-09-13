package main

import "testing"

// A backend 4xx must carry actionable guidance, not just the raw JSON, so a blind agent can
// tell "this feature/device isn't on my account" from "I built the request wrong" and stop
// retrying. Observed in the navigation test: raw `HTTP 403: {json}` drove long probing loops
// (issue #75).
func TestHTTPErrorGuidance(t *testing.T) {
	body := []byte(`{"statusCode":403,"errors":[{"code":403,"message":"Forbidden","details":"User has no permissions on this serviceId"}]}`)
	err := httpError(403, body)
	if err == nil {
		t.Fatal("httpError(403) returned nil")
	}
	msg := err.Error()
	// keeps the raw body for debugging
	if !contains(msg, "User has no permissions") {
		t.Errorf("403 message dropped the raw backend body:\n%s", msg)
	}
	// points at describe (where unavailable ops are marked) and the target flag
	for _, want := range []string{"describe", "service-id"} {
		if !contains(msg, want) {
			t.Errorf("403 guidance should mention %q:\n%s", want, msg)
		}
	}
	// 401 guidance is about the session/token, not the target
	a := httpError(401, []byte(`{"code":401}`)).Error()
	if !contains(a, "auth refresh") {
		t.Errorf("401 guidance should mention re-auth:\n%s", a)
	}
	// 404 and 400 each get their own hint
	if !contains(httpError(404, []byte(`{}`)).Error(), "does not exist") {
		t.Errorf("404 guidance missing")
	}
	if !contains(httpError(400, []byte(`{}`)).Error(), "request") {
		t.Errorf("400 guidance missing")
	}
	// an unclassified status still errors with the body
	if httpError(503, []byte(`x`)) == nil {
		t.Error("httpError(503) should still return an error")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
