package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/ejc3/safe_cli/internal/signing"
)

func TestDoSendsRawIDTokenAndHeaders(t *testing.T) {
	var gotAuth, gotSrc, gotVer, gotTxn, gotUA string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotSrc = r.Header.Get("x-source-app")
		gotVer = r.Header.Get("x-mobile-app-version")
		gotTxn = r.Header.Get("x-transaction-id")
		gotUA = r.Header.Get("user-agent")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("THE.ID.TOKEN")
	c.BaseURL = srv.URL
	resp, err := c.Do(context.Background(), "POST", "/x", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || string(resp.Body) != `{"ok":true}` {
		t.Fatalf("resp = %d %s", resp.Status, resp.Body)
	}
	// Authorization is the raw id_token — NOT "Bearer ...".
	if gotAuth != "THE.ID.TOKEN" {
		t.Errorf("Authorization = %q, want raw id_token with no Bearer prefix", gotAuth)
	}
	if gotSrc != "AndroidMAPP" || gotVer != AppVersion || gotUA != "okhttp/4.12.0" {
		t.Errorf("headers: src=%q ver=%q ua=%q", gotSrc, gotVer, gotUA)
	}
	// x-transaction-id now matches the app: a decimal integer up to 40 digits.
	if !regexp.MustCompile(`^\d{1,40}$`).MatchString(gotTxn) {
		t.Errorf("x-transaction-id = %q, want a decimal (<=40 digits)", gotTxn)
	}
	if gotBody != `{"a":1}` {
		t.Errorf("body = %q", gotBody)
	}
}

func TestDoRequiresToken(t *testing.T) {
	c := New("")
	if _, err := c.Do(context.Background(), "GET", "/x", nil); err == nil {
		t.Fatal("expected an error when no id_token is set")
	}
}

func TestSignedDoSetsSelfConsistentSignature(t *testing.T) {
	const (
		key     = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		appUUID = "c9ce8abc-2e84-3e8e-81bd-07557dd60015"
	)
	var h http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"OTP_SENT"}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL
	resp, err := c.SignedDo(context.Background(), "POST", "/auth/otp", []byte(`{"mdn":"5551234567"}`), key, appUUID)
	if err != nil {
		t.Fatalf("SignedDo: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status = %d", resp.Status)
	}
	// Unauthenticated: no Authorization header on device-auth calls.
	if got := h.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want none on a signed device-auth call", got)
	}
	if got := h.Get("x-appuuid"); got != appUUID {
		t.Errorf("x-appuuid = %q, want %q", got, appUUID)
	}
	if h.Get("x-source-app") != "AndroidMAPP" || h.Get("x-mobile-app-version") != signing.AppVersion {
		t.Errorf("app headers: src=%q ver=%q", h.Get("x-source-app"), h.Get("x-mobile-app-version"))
	}
	if !regexp.MustCompile(`^\d{1,40}$`).MatchString(h.Get("x-transaction-id")) {
		t.Errorf("x-transaction-id = %q", h.Get("x-transaction-id"))
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(h.Get("x-trace-transaction-id")) {
		t.Errorf("x-trace-transaction-id = %q, want uuid v4", h.Get("x-trace-transaction-id"))
	}
	// The signature must recompute from the very headers that were sent.
	want := signing.Signature(key, "POST", h.Get("x-transaction-id"), h.Get("x-timestamp"), h.Get("x-appuuid"))
	if got := h.Get("x-signature"); got != want {
		t.Errorf("x-signature = %q, not self-consistent (want %q)", got, want)
	}
}

func TestSignedDoRequiresKeyAndUUID(t *testing.T) {
	c := New("")
	if _, err := c.SignedDo(context.Background(), "POST", "/x", nil, "", "uuid"); err == nil {
		t.Error("want error when signing key is empty")
	}
	if _, err := c.SignedDo(context.Background(), "POST", "/x", nil, "key", ""); err == nil {
		t.Error("want error when app uuid is empty")
	}
}
