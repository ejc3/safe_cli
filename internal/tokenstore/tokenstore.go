// Package tokenstore persists the SafePath auth token set (as returned by the
// frisco token endpoint) to a 0600 file under the user's config dir, and exposes
// the online/offline token pair the client and refresh use.
package tokenstore

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jwtClaimInt parses an (unverified) JWT and returns an integer claim such as
// "exp" or "iat". The token is a bearer artifact we already hold, so we read its
// payload for timing without verifying the signature.
func jwtClaimInt(token, claim string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return 0, false
	}
	if v, ok := m[claim].(float64); ok {
		return int64(v), true
	}
	return 0, false
}

// Token is one entry of the frisco token response.
type Token struct {
	IDToken         string `json:"id_token"`
	AccessToken     string `json:"access_token,omitempty"`
	RefreshToken    string `json:"refresh_token,omitempty"`
	TokenType       string `json:"token_type,omitempty"`
	ExpiresIn       int    `json:"expires_in,omitempty"`
	FriscoTokenType string `json:"frisco_token_type,omitempty"` // "online" | "offline"
	FidoGUID        string `json:"fido_guid,omitempty"`
	ObtainedAt      int64  `json:"obtained_at,omitempty"` // unix seconds; set on save
}

// TokenSet is the persisted auth material.
type TokenSet struct {
	MDN       string  `json:"mdn,omitempty"`
	AppUUID   string  `json:"app_uuid,omitempty"`
	Tokens    []Token `json:"tokens"`
	AuthLevel string  `json:"authLevel,omitempty"`
}

// byType returns the first token with the given frisco_token_type.
func (s *TokenSet) byType(t string) (Token, bool) {
	for _, tok := range s.Tokens {
		if tok.FriscoTokenType == t {
			return tok, true
		}
	}
	return Token{}, false
}

// Online returns the short-lived token used for API calls.
func (s *TokenSet) Online() (Token, bool) { return s.byType("online") }

// Offline returns the long-lived token used to refresh.
func (s *TokenSet) Offline() (Token, bool) { return s.byType("offline") }

// IDToken returns the id_token to send as the Authorization header (online first).
func (s *TokenSet) IDToken() (string, bool) {
	if t, ok := s.Online(); ok && t.IDToken != "" {
		return t.IDToken, true
	}
	if t, ok := s.Offline(); ok && t.IDToken != "" {
		return t.IDToken, true
	}
	return "", false
}

// Expired reports whether the token is past (or within skew of) its expiry.
// It prefers the id_token JWT's own `exp` (absolute, robust to any delay between
// issuance and import); only if that is unavailable does it fall back to
// ObtainedAt+ExpiresIn, and if neither is known it returns false.
func (t Token) Expired(skew time.Duration) bool {
	if exp, ok := jwtClaimInt(t.IDToken, "exp"); ok {
		return time.Now().Add(skew).After(time.Unix(exp, 0))
	}
	if t.ObtainedAt == 0 || t.ExpiresIn == 0 {
		return false // unknown; let the server decide
	}
	deadline := time.Unix(t.ObtainedAt, 0).Add(time.Duration(t.ExpiresIn) * time.Second)
	return time.Now().Add(skew).After(deadline)
}

// Store is a token file location.
type Store struct{ Path string }

// DefaultStore resolves ~/.config/safe_cli/tokens.json (honoring XDG_CONFIG_HOME).
func DefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(dir, "safe_cli", "tokens.json")}, nil
}

// Save writes the set with 0600 perms, stamping ObtainedAt on unstamped tokens.
func (s *Store) Save(ts *TokenSet, now time.Time) error {
	for i := range ts.Tokens {
		if ts.Tokens[i].ObtainedAt == 0 {
			// Use the JWT's real issuance time so a delayed import can't extend the
			// apparent lifetime; fall back to now only when the token isn't a JWT.
			if iat, ok := jwtClaimInt(ts.Tokens[i].IDToken, "iat"); ok {
				ts.Tokens[i].ObtainedAt = iat
			} else {
				ts.Tokens[i].ObtainedAt = now.Unix()
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	// Write via a temp file + rename so a crash can't leave a half-written token file.
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Load reads the set. Returns (nil, os.ErrNotExist) when no token file exists yet.
func (s *Store) Load() (*TokenSet, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	var ts TokenSet
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, fmt.Errorf("parse token file %s: %w", s.Path, err)
	}
	return &ts, nil
}

// Delete removes the token file (logout). Missing file is not an error.
func (s *Store) Delete() error {
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
