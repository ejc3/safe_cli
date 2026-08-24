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

// newAppRequest must set the app's exact wire headers: lowercase x-* names (NOT Go's
// canonicalized X-Source-App), and the app's values and User-Agent. This is what lets
// the server see no delta between us and the real app.
func TestNewAppRequestMatchesAppHeaders(t *testing.T) {
	c := New("tok")
	req, err := c.newAppRequest(context.Background(), "GET", "/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Exact lowercase names present (direct map access does not canonicalize).
	want := map[string]string{
		"x-source-app":         "AndroidMAPP",
		"x-mobile-app-version": "8.101.30",
		"User-Agent":           "okhttp/4.12.0",
		"Accept":               "*/*",
		"Content-Type":         "application/json",
	}
	for k, v := range want {
		got := req.Header[k]
		if len(got) != 1 || got[0] != v {
			t.Errorf("header %q = %v, want [%q]", k, got, v)
		}
	}
	if req.Header["x-transaction-id"] == nil { //nolint:staticcheck // SA1008: intentional non-canonical key — verifies the app's lowercase wire header
		t.Error("missing x-transaction-id")
	}
	// The canonicalized spellings must NOT also be present (that would double the header
	// and reveal us as a non-app client).
	for _, canon := range []string{"X-Source-App", "X-Mobile-App-Version", "X-Transaction-Id"} {
		if req.Header[canon] != nil {
			t.Errorf("header was canonicalized to %q — differs from the app's lowercase wire name", canon)
		}
	}
}

// DoH keeps the caller's exact lowercase per-op header names (x-fp-identifier-*) rather
// than canonicalizing them.
func TestDoHKeepsLowercasePerOpHeaders(t *testing.T) {
	c := New("tok")
	req, err := c.newAppRequest(context.Background(), "GET", "/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	setRaw(req.Header, "Authorization", c.IDToken)
	for k, v := range map[string]string{"x-fp-identifier-target-serviceid": "svc"} {
		setRaw(req.Header, k, v)
	}
	if req.Header["x-fp-identifier-target-serviceid"] == nil || req.Header["X-Fp-Identifier-Target-Serviceid"] != nil { //nolint:staticcheck // SA1008: intentional — asserts lowercase kept, canonical form absent
		t.Errorf("per-op header casing not preserved: %v", req.Header)
	}
	if got := req.Header["Authorization"]; len(got) != 1 || got[0] != "tok" {
		t.Errorf("Authorization = %v", got)
	}
}

// A caller-supplied reserved header (user-agent) replaces the default instead of leaking a
// lowercase duplicate; an app-specific header keeps its exact lowercase casing.
func TestSetHeaderCanonicalizesReserved(t *testing.T) {
	h := http.Header{}
	setRaw(h, "User-Agent", "okhttp/4.12.0") // the app default
	setHeader(h, "user-agent", "custom")     // caller override via --header
	if h["user-agent"] != nil {              //nolint:staticcheck // SA1008: asserting no lowercase duplicate leaked
		t.Errorf("lowercase user-agent duplicate leaked: %v", h)
	}
	if got := h.Get("User-Agent"); got != "custom" {
		t.Errorf("User-Agent = %q, want custom (replaced)", got)
	}
	setHeader(h, "x-fp-identifier-target-serviceid", "svc")
	if h["x-fp-identifier-target-serviceid"] == nil { //nolint:staticcheck // SA1008: verifies app casing preserved
		t.Errorf("app-specific header casing not preserved: %v", h)
	}
}

// A caller override with different casing than an app header replaces it (no duplicate,
// case-insensitively) rather than sending both values.
func TestSetHeaderDedupsCaseInsensitively(t *testing.T) {
	h := http.Header{}
	setRaw(h, "x-transaction-id", "app-generated") // the app default (lowercase)
	setHeader(h, "X-Transaction-Id", "caller")     // caller override, different casing
	// Exactly one value across all casings.
	var vals []string
	for k := range h {
		if http.CanonicalHeaderKey(k) == "X-Transaction-Id" {
			vals = append(vals, h[k]...)
		}
	}
	if len(vals) != 1 || vals[0] != "caller" {
		t.Errorf("x-transaction-id values = %v, want exactly [caller]", vals)
	}
}

// Accept-Encoding must go through the reserved (canonicalized) path so the transport
// sees the caller's override and does not also add gzip (Codex #21).
func TestSetHeaderCanonicalizesAcceptEncoding(t *testing.T) {
	h := http.Header{}
	setHeader(h, "accept-encoding", "identity")
	if h["accept-encoding"] != nil { //nolint:staticcheck // SA1008: verifies no lowercase raw key
		t.Errorf("accept-encoding stored under raw lowercase key: %v", h)
	}
	if got := h.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("Accept-Encoding = %q, want identity", got)
	}
}

// A standard header the transport inspects by canonical key (Range) is canonicalized, not
// stored raw — otherwise Transport would add gzip and could corrupt a partial response.
func TestSetHeaderCanonicalizesRangeAndReferer(t *testing.T) {
	h := http.Header{}
	setHeader(h, "range", "bytes=0-99")
	if h["range"] != nil { //nolint:staticcheck // SA1008: verifies no raw lowercase key
		t.Errorf("range stored under raw lowercase key: %v", h)
	}
	if got := h.Get("Range"); got != "bytes=0-99" {
		t.Errorf("Range = %q, want bytes=0-99", got)
	}
	setHeader(h, "referer", "https://x")
	if h["referer"] != nil { //nolint:staticcheck // SA1008: verifies no raw lowercase key
		t.Errorf("referer stored raw: %v", h)
	}
	if h.Get("Referer") != "https://x" {
		t.Errorf("Referer = %q", h.Get("Referer"))
	}
	// app-specific still preserved
	setHeader(h, "x-source-app", "AndroidMAPP")
	if h["x-source-app"] == nil { //nolint:staticcheck // SA1008: verifies app casing kept
		t.Error("x-source-app casing not preserved")
	}
}

// Custom descriptor-declared headers (app-name, addressType, gizmo-device-model, …) must
// keep their exact wire casing — they are not standard headers, so Header.Set would rewrite
// them (app-name -> App-Name) and diverge from the app (Codex #21 post-merge).
func TestSetHeaderPreservesCustomHeaders(t *testing.T) {
	for _, k := range []string{"app-name", "addressType", "gizmo-device-model", "user-profile-id", "crash-date-time"} {
		h := http.Header{}
		setHeader(h, k, "v")
		if h[k] == nil { //nolint:staticcheck // SA1008: verifies the exact custom key is kept
			t.Errorf("custom header %q was not preserved raw: %v", k, h)
		}
		if h[http.CanonicalHeaderKey(k)] != nil && http.CanonicalHeaderKey(k) != k {
			t.Errorf("custom header %q was canonicalized to %q", k, http.CanonicalHeaderKey(k))
		}
	}
}
