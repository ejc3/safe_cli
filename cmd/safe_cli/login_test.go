package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/oauth"
)

// mockBackend routes the three auth-flow endpoints from the descriptor.
func mockBackend(t *testing.T, ep map[string]string, hits map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path] = true
		switch r.URL.Path {
		case ep["otp_request"]:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"OTP_SENT"}`))
		case ep["otp_validate"]:
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`{"state":"AM_LOGIN_PAGE","tokens":[{"token_type":"login_recom_token","id_token":"RECOM","expires_in":1800}]}`))
		case ep["user_auth_token"]:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tokens":[` +
				`{"frisco_token_type":"online","id_token":"IDon","access_token":"AC","refresh_token":"RTon","expires_in":1800},` +
				`{"frisco_token_type":"offline","id_token":"IDoff","refresh_token":"RToff","expires_in":86400}]}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testLoginDeps(t *testing.T, srvURL string, stdin string, opened *string) loginDeps {
	t.Helper()
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	cl := client.New("")
	cl.BaseURL = srvURL
	return loginDeps{
		Client:  cl,
		Desc:    d,
		Key:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AppUUID: "00000000-0000-4000-8000-000000000000",
		In:      bufio.NewReader(strings.NewReader(stdin)),
		Out:     &strings.Builder{},
		OpenURL: func(u string) error { *opened = u; return nil },
		Now:     time.Unix(1700000000, 0),
		genPKCE: func() (oauth.PKCE, error) { return oauth.PKCE{Verifier: "VER", Challenge: "CHAL"}, nil },
	}
}

func TestRunLoginHappyPath(t *testing.T) {
	d, _ := descriptor.Default()
	ep := d.Auth.Endpoints
	hits := map[string]bool{}
	srv := mockBackend(t, ep, hits)
	defer srv.Close()

	settings := oauth.FromDescriptor(d.Auth)
	// State is lenient now (the backend rebinds it), so any state on the paste works.
	paste := settings.RedirectURI + "?code=AUTHCODE&state=backend_rebound"
	var opened string
	deps := testLoginDeps(t, srv.URL, "5551234567\n071480\n"+paste+"\n", &opened)

	ts, err := runLogin(context.Background(), deps)
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	for _, p := range []string{ep["otp_request"], ep["otp_validate"], ep["user_auth_token"]} {
		if !hits[p] {
			t.Errorf("endpoint not hit: %s", p)
		}
	}
	// The authorize URL carries the confirmed-live params — not the old login_recom_token.
	for _, want := range []string{
		"code_challenge=CHAL",
		"identity_provider=vz-am-provider",
		"frisco_token_type=online",
		"x-source-app=AndroidMAPP",
		"app_uuid=00000000-0000-4000-8000-000000000000",
	} {
		if !strings.Contains(opened, want) {
			t.Errorf("authorize URL missing %q: %s", want, opened)
		}
	}
	if strings.Contains(opened, "login_recom_token") {
		t.Errorf("authorize URL must not carry login_recom_token: %s", opened)
	}
	off, ok := ts.Offline()
	if !ok || off.RefreshToken != "RToff" {
		t.Errorf("offline refresh_token = %q ok=%v, want RToff", off.RefreshToken, ok)
	}
	if ts.MDN != "5551234567" || ts.AppUUID != "00000000-0000-4000-8000-000000000000" {
		t.Errorf("mdn=%q appuuid=%q", ts.MDN, ts.AppUUID)
	}
}

// A supplied OTP (--otp) resumes a login with an already-delivered code: it must NOT hit
// the otp_request endpoint (no new SMS) but must still validate the code and exchange. This
// lets a retry reuse a code the backend already sent instead of forcing a fresh one.
func TestRunLoginWithSuppliedOTPSkipsRequest(t *testing.T) {
	d, _ := descriptor.Default()
	ep := d.Auth.Endpoints
	hits := map[string]bool{}
	srv := mockBackend(t, ep, hits)
	defer srv.Close()

	settings := oauth.FromDescriptor(d.Auth)
	paste := settings.RedirectURI + "?code=AUTHCODE&state=x"
	var opened string
	// stdin carries ONLY the pasted redirect — no OTP line, since deps.OTP supplies it.
	deps := testLoginDeps(t, srv.URL, paste+"\n", &opened)
	deps.MDN = "5551234567"
	deps.OTP = "071480"

	ts, err := runLogin(context.Background(), deps)
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if hits[ep["otp_request"]] {
		t.Error("otp_request must NOT be hit when --otp supplies an already-sent code")
	}
	if !hits[ep["otp_validate"]] {
		t.Error("otp_validate must still be hit to validate the supplied code")
	}
	if off, ok := ts.Offline(); !ok || off.RefreshToken != "RToff" {
		t.Errorf("offline refresh_token = %q ok=%v, want RToff", off.RefreshToken, ok)
	}
}

// A redirect to the wrong target (not our registered vsfapp:// URI) must abort before any
// code is exchanged — even though the state check itself is lenient.
func TestRunLoginRejectsBadRedirectTarget(t *testing.T) {
	d, _ := descriptor.Default()
	srv := mockBackend(t, d.Auth.Endpoints, map[string]bool{})
	defer srv.Close()

	paste := "vsfapp://evil.app/signin?code=AUTHCODE&state=x"
	var opened string
	deps := testLoginDeps(t, srv.URL, "5551234567\n071480\n"+paste+"\n", &opened)

	if _, err := runLogin(context.Background(), deps); err == nil {
		t.Fatal("want an error on a redirect to the wrong target")
	}
}

// When WaitRedirect is wired (the macOS scheme handler), the redirect is captured
// automatically instead of prompting for a paste.
func TestRunLoginSchemeCapture(t *testing.T) {
	d, _ := descriptor.Default()
	ep := d.Auth.Endpoints
	hits := map[string]bool{}
	srv := mockBackend(t, ep, hits)
	defer srv.Close()

	settings := oauth.FromDescriptor(d.Auth)
	var opened string
	// stdin only answers the device-OTP prompts; the redirect comes from WaitRedirect.
	deps := testLoginDeps(t, srv.URL, "5551234567\n071480\n", &opened)
	deps.WaitRedirect = func(context.Context) (string, error) {
		return settings.RedirectURI + "?code=AUTHCODE&state=captured", nil
	}

	ts, err := runLogin(context.Background(), deps)
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if !hits[ep["user_auth_token"]] {
		t.Error("exchange endpoint not hit via scheme capture")
	}
	if _, ok := ts.Offline(); !ok {
		t.Error("no offline token after scheme-captured login")
	}
}

// The Windows opener must not route the authorize URL through cmd.exe, whose `&`
// handling would split the URL; the URL must reach the opener as one intact arg.
func TestBrowserCommandNoShell(t *testing.T) {
	const u = "https://api/authorize?client_id=a&state=b&code_challenge=c"
	name, args := browserCommand("windows", u)
	if name == "cmd" {
		t.Error("windows opener uses cmd.exe; the URL's & would split it")
	}
	intact := false
	for _, a := range args {
		if a == u {
			intact = true
		}
	}
	if !intact {
		t.Errorf("URL not passed intact as a single argv element: %v", args)
	}
	if n, _ := browserCommand("darwin", u); n != "open" {
		t.Errorf("darwin opener = %q, want open", n)
	}
	if n, _ := browserCommand("linux", u); n != "xdg-open" {
		t.Errorf("linux opener = %q, want xdg-open", n)
	}
}

func TestWriteLoginResult(t *testing.T) {
	var jb strings.Builder
	if err := writeLoginResult(&jb, true, "5551234567", "/cfg/tokens.json"); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(jb.String()), &m); err != nil {
		t.Fatalf("--json output is not JSON: %v (%q)", err, jb.String())
	}
	if m["status"] != "ok" || m["mdn"] != "5551234567" || m["tokens_path"] != "/cfg/tokens.json" {
		t.Errorf("json result = %v", m)
	}

	var pb strings.Builder
	if err := writeLoginResult(&pb, false, "5551234567", "/cfg/tokens.json"); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(pb.String()), "{") {
		t.Errorf("non-json result should be a human line, got %q", pb.String())
	}
}
