// Package oauth builds the SafePath authorization-code + PKCE request and parses the
// redirect the browser lands on. It owns the PKCE secret (code_verifier), which is what
// makes the captured authorization code safe to carry over a custom URI scheme: the
// code is useless without the verifier, which never leaves this process.
//
// The OAuth client parameters (client_id, redirect_uri, scope, authorize path) are NOT
// hardcoded here — they come from the protocol descriptor (the single source of truth)
// via Settings/FromDescriptor, so updating the descriptor updates the live flow.
//
// The Akamai-protected hosted login itself is completed by a real browser (see
// docs/PROCESS.md §9/§9a); this package only constructs the URL the browser opens and
// reads the code back out of the redirect it ends on.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// Settings are the OAuth client parameters, sourced from the descriptor.
type Settings struct {
	ClientID      string
	RedirectURI   string // registered redirect (vsfapp://…/signin)
	Scope         string
	AuthorizePath string
}

// FromDescriptor reads the OAuth settings out of the descriptor's auth block.
func FromDescriptor(a descriptor.Auth) Settings {
	return Settings{
		ClientID:      a.ClientID,
		RedirectURI:   a.RedirectURI,
		Scope:         a.Scope,
		AuthorizePath: a.Endpoints["authorize"],
	}
}

// PKCE is a code_verifier and its S256 code_challenge (RFC 7636).
type PKCE struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a fresh PKCE pair: a 43-char base64url code_verifier (256 bits
// of entropy) and its SHA-256 challenge.
func GeneratePKCE() (PKCE, error) {
	v, err := randB64URL(32)
	if err != nil {
		return PKCE{}, err
	}
	sum := sha256.Sum256([]byte(v))
	return PKCE{Verifier: v, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// State returns a random opaque state value for CSRF protection.
func State() (string, error) { return randB64URL(24) }

func randB64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURL builds the frisco authorize URL to open in the browser. baseURL is the
// backend origin (client.DefaultBaseURL). redirectURI overrides s.RedirectURI when
// non-empty — so the live run can try the cleaner RFC 8252 loopback
// (http://127.0.0.1:PORT/…) first and fall back to the registered vsfapp:// scheme.
// extra carries flow-specific params observed in the capture (e.g. the
// login_recom_token from device-OTP validation).
func (s Settings) AuthorizeURL(baseURL, redirectURI, state, challenge string, extra map[string]string) string {
	if redirectURI == "" {
		redirectURI = s.RedirectURI
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", s.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", s.Scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	for k, v := range extra {
		q.Set(k, v)
	}
	return strings.TrimRight(baseURL, "/") + s.AuthorizePath + "?" + q.Encode()
}

// ParseRedirect extracts the authorization code from the URL the browser ends on,
// validating that it targets wantRedirect (the registered vsfapp:// scheme, or a
// loopback callback) and that state matches (CSRF check).
func ParseRedirect(redirect, wantRedirect, wantState string) (code string, err error) {
	u, err := url.Parse(strings.TrimSpace(redirect))
	if err != nil {
		return "", fmt.Errorf("parse redirect url: %w", err)
	}
	want, err := url.Parse(wantRedirect)
	if err != nil {
		return "", fmt.Errorf("parse expected redirect: %w", err)
	}
	if u.Scheme != want.Scheme || u.Host != want.Host || u.Path != want.Path {
		return "", fmt.Errorf("unexpected redirect target %q (want %s)", redirect, wantRedirect)
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("authorization error: %s: %s", e, q.Get("error_description"))
	}
	if got := q.Get("state"); got != wantState {
		return "", fmt.Errorf("state mismatch (possible CSRF): got %q", got)
	}
	if code = q.Get("code"); code == "" {
		return "", fmt.Errorf("no authorization code in redirect")
	}
	return code, nil
}
