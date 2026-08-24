package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"regexp"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

func testSettings(t *testing.T) Settings {
	t.Helper()
	d, err := descriptor.Default()
	if err != nil {
		t.Fatalf("descriptor.Default: %v", err)
	}
	return FromDescriptor(d.Auth)
}

func TestGeneratePKCE(t *testing.T) {
	p, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	// RFC 7636: verifier is 43-128 chars of the unreserved set; base64url of 32 bytes = 43.
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(p.Verifier) {
		t.Errorf("verifier = %q", p.Verifier)
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Errorf("challenge = %q, want %q", p.Challenge, want)
	}
	if p2, _ := GeneratePKCE(); p2.Verifier == p.Verifier {
		t.Error("two GeneratePKCE calls produced the same verifier")
	}
}

// The OAuth settings must come from the descriptor, not hardcoded here.
func TestFromDescriptorMatchesDescriptor(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	s := FromDescriptor(d.Auth)
	if s.ClientID != d.Auth.ClientID || s.RedirectURI != d.Auth.RedirectURI ||
		s.Scope != d.Auth.Scope || s.AuthorizePath != d.Auth.Endpoints["authorize"] {
		t.Fatalf("settings %+v do not match descriptor auth block", s)
	}
	if s.AuthorizePath == "" {
		t.Fatal("descriptor has no authorize endpoint")
	}
}

func TestAuthorizeURL(t *testing.T) {
	s := testSettings(t)
	got := s.AuthorizeURL("https://api.example.com/", "", "STATE123", "CHAL456",
		map[string]string{"login_recom_token": "RECOM"})
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("bad url: %v", err)
	}
	if u.Path != s.AuthorizePath {
		t.Errorf("path = %q, want %q", u.Path, s.AuthorizePath)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"response_type":         "code",
		"client_id":             s.ClientID,
		"redirect_uri":          s.RedirectURI,
		"scope":                 s.Scope,
		"code_challenge":        "CHAL456",
		"code_challenge_method": "S256",
		"state":                 "STATE123",
		"login_recom_token":     "RECOM",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
}

// A loopback redirect override replaces the registered vsfapp:// value.
func TestAuthorizeURLLoopbackOverride(t *testing.T) {
	s := testSettings(t)
	const cb = "http://127.0.0.1:8123/callback"
	got := s.AuthorizeURL("https://api.example.com", cb, "S", "C", nil)
	u, _ := url.Parse(got)
	if u.Query().Get("redirect_uri") != cb {
		t.Errorf("redirect_uri = %q, want the loopback override", u.Query().Get("redirect_uri"))
	}
}

func TestParseRedirectVsfapp(t *testing.T) {
	s := testSettings(t)
	code, err := ParseRedirect(s.RedirectURI+"?code=AUTHCODE&state=S", s.RedirectURI, "S")
	if err != nil {
		t.Fatalf("ParseRedirect: %v", err)
	}
	if code != "AUTHCODE" {
		t.Errorf("code = %q", code)
	}
}

func TestParseRedirectLoopback(t *testing.T) {
	const cb = "http://127.0.0.1:8123/callback"
	code, err := ParseRedirect(cb+"?state=S&code=XYZ", cb, "S")
	if err != nil || code != "XYZ" {
		t.Fatalf("loopback: code=%q err=%v", code, err)
	}
}

func TestParseRedirectRejects(t *testing.T) {
	s := testSettings(t)
	cases := []struct{ name, redirect, want, state string }{
		{"state-mismatch", s.RedirectURI + "?code=C&state=WRONG", s.RedirectURI, "S"},
		{"error-param", s.RedirectURI + "?error=access_denied&state=S", s.RedirectURI, "S"},
		{"no-code", s.RedirectURI + "?state=S", s.RedirectURI, "S"},
		{"wrong-target", "vsfapp://evil.app/signin?code=C&state=S", s.RedirectURI, "S"},
		{"wrong-scheme", "https://phish.example/signin?code=C&state=S", s.RedirectURI, "S"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseRedirect(c.redirect, c.want, c.state); err == nil {
				t.Errorf("want error for %s", c.name)
			}
		})
	}
}
