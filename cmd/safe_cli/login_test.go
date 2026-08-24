package main

import (
	"bufio"
	"context"
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

func testLoginDeps(t *testing.T, srvURL string, stdin string, state string, opened *string) loginDeps {
	t.Helper()
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	cl := client.New("")
	cl.BaseURL = srvURL
	return loginDeps{
		Client:   cl,
		Desc:     d,
		Key:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AppUUID:  "00000000-0000-4000-8000-000000000000",
		In:       bufio.NewReader(strings.NewReader(stdin)),
		Out:      &strings.Builder{},
		OpenURL:  func(u string) error { *opened = u; return nil },
		Now:      time.Unix(1700000000, 0),
		genState: func() (string, error) { return state, nil },
		genPKCE:  func() (oauth.PKCE, error) { return oauth.PKCE{Verifier: "VER", Challenge: "CHAL"}, nil },
	}
}

func TestRunLoginHappyPath(t *testing.T) {
	d, _ := descriptor.Default()
	ep := d.Auth.Endpoints
	hits := map[string]bool{}
	srv := mockBackend(t, ep, hits)
	defer srv.Close()

	const state = "TESTSTATE123"
	settings := oauth.FromDescriptor(d.Auth)
	paste := settings.RedirectURI + "?code=AUTHCODE&state=" + state
	var opened string
	deps := testLoginDeps(t, srv.URL, "5551234567\n071480\n"+paste+"\n", state, &opened)

	ts, err := runLogin(context.Background(), deps)
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	for _, p := range []string{ep["otp_request"], ep["otp_validate"], ep["user_auth_token"]} {
		if !hits[p] {
			t.Errorf("endpoint not hit: %s", p)
		}
	}
	for _, want := range []string{"state=" + state, "code_challenge=CHAL", "login_recom_token=RECOM"} {
		if !strings.Contains(opened, want) {
			t.Errorf("authorize URL missing %q: %s", want, opened)
		}
	}
	off, ok := ts.Offline()
	if !ok || off.RefreshToken != "RToff" {
		t.Errorf("offline refresh_token = %q ok=%v, want RToff", off.RefreshToken, ok)
	}
	if ts.MDN != "5551234567" || ts.AppUUID != "00000000-0000-4000-8000-000000000000" {
		t.Errorf("mdn=%q appuuid=%q", ts.MDN, ts.AppUUID)
	}
}

func TestRunLoginRejectsBadRedirectState(t *testing.T) {
	d, _ := descriptor.Default()
	srv := mockBackend(t, d.Auth.Endpoints, map[string]bool{})
	defer srv.Close()

	settings := oauth.FromDescriptor(d.Auth)
	// Paste carries the WRONG state — a CSRF mismatch must abort the login.
	paste := settings.RedirectURI + "?code=AUTHCODE&state=WRONG"
	var opened string
	deps := testLoginDeps(t, srv.URL, "5551234567\n071480\n"+paste+"\n", "REALSTATE", &opened)

	if _, err := runLogin(context.Background(), deps); err == nil {
		t.Fatal("want an error on a state mismatch in the pasted redirect")
	}
}
