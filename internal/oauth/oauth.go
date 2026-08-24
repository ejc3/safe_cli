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

// Request-identity constants carried on the authorize URL. They mirror the app's values
// (kept in sync with internal/client): the frisco backend answers 500 without
// identity_provider, and expects the x-source-app / x-mobile-app-version pair.
const (
	sourceApp        = "AndroidMAPP"
	appVersion       = "8.101.30"
	identityProvider = "vz-am-provider"
	friscoTokenType  = "online"
)

// newUUID is overridable in tests so the generated transaction ids (and thus the state)
// are deterministic.
var newUUID = uuidV4

func uuidV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// AuthorizeURL builds the frisco authorize URL to open in the browser, with the exact
// parameter set confirmed against the live backend (docs/PROCESS.md §9). Besides the
// standard OAuth + PKCE params it MUST carry identity_provider (omitting it yields a 500),
// frisco_token_type, the app_uuid, and the request-identity params x-source-app /
// x-mobile-app-version / x-transaction-id / x-trace-transaction-id, with
// state = "<trace>_<xact>". baseURL is the backend origin; redirectURI overrides
// s.RedirectURI when non-empty. It returns the URL and the state it generated — though the
// backend rebinds state server-side per app_uuid, so callers must treat the returned
// redirect's state loosely (see ParseRedirect).
func (s Settings) AuthorizeURL(baseURL, redirectURI, appUUID, challenge string) (string, string, error) {
	if redirectURI == "" {
		redirectURI = s.RedirectURI
	}
	trace, err := newUUID()
	if err != nil {
		return "", "", err
	}
	xact, err := newUUID()
	if err != nil {
		return "", "", err
	}
	state := trace + "_" + xact
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", s.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", s.Scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("frisco_token_type", friscoTokenType)
	q.Set("identity_provider", identityProvider)
	q.Set("app_uuid", appUUID)
	q.Set("x-source-app", sourceApp)
	q.Set("x-mobile-app-version", appVersion)
	q.Set("x-transaction-id", xact)
	q.Set("x-trace-transaction-id", trace)
	q.Set("state", state)
	return strings.TrimRight(baseURL, "/") + s.AuthorizePath + "?" + q.Encode(), state, nil
}

// ParseRedirect extracts the authorization code from the URL the browser ends on,
// validating that it targets wantRedirect (the registered vsfapp:// scheme, or a loopback
// callback). If wantState is non-empty the redirect's state must match it; pass "" to skip
// that check — the frisco backend rebinds state server-side per app_uuid, so the returned
// state does not reliably equal the one we sent. Security does not rest on state here: the
// code is worthless without the PKCE code_verifier (which never leaves this process) and,
// at exchange, the device-OTP recom token.
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
	if wantState != "" {
		if got := q.Get("state"); got != wantState {
			return "", fmt.Errorf("state mismatch (possible CSRF): got %q", got)
		}
	}
	if code = q.Get("code"); code == "" {
		return "", fmt.Errorf("no authorization code in redirect")
	}
	return code, nil
}
