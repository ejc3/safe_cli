// Package client is the SafePath (Verizon Family) HTTP client. It has two request
// paths:
//
//   - Do: id_token-authenticated calls. The app sends the raw id_token as the
//     Authorization header (no "Bearer " prefix). Covers the config/content surface
//     and `raw`.
//   - SignedDo: the device-auth endpoints (OTP send/validate, token refresh). These
//     are unauthenticated but require the x-signature HMAC over request metadata
//     (see docs/PROCESS.md §11). The caller supplies the app signing key
//     (apkkey.ResolveKey) and this install's app UUID.
//   - ExchangeCode: the OAuth code→token exchange, which is neither authenticated nor
//     signed (docs/PROCESS.md §9 step 6, §10) — just the mundane app headers.
//
// NOTE: the parental-control operations use AWS SigV4 (Cognito temp creds), not the
// id_token — see docs/PROCESS.md §10. Those are handled by a separate signer added
// in a later phase.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ejc3/safe_cli/internal/signing"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

const (
	// DefaultBaseURL is the frisco backend.
	DefaultBaseURL = "https://api.prd.vsf.aws.vz-connect.com"
	// AppVersion matches the analyzed client; some endpoints echo/verify it.
	AppVersion = "8.101.30"
	userAgent  = "okhttp/4.12.0"
	sourceApp  = "AndroidMAPP"
)

// Client issues authenticated requests against the SafePath backend.
type Client struct {
	BaseURL    string
	IDToken    string
	AppVersion string
	HTTP       *http.Client
}

// New builds a client with sane defaults.
func New(idToken string) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		IDToken:    idToken,
		AppVersion: AppVersion,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Response is a decoded HTTP response.
type Response struct {
	Status int
	Body   []byte
}

// newAppRequest builds a request carrying the app's mundane headers (source-app,
// version, a fresh x-transaction-id, user-agent, accept, content-type) — the common
// base for Do and ExchangeCode.
func (c *Client) newAppRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return nil, err
	}
	txn, err := signing.TransactionID()
	if err != nil {
		return nil, fmt.Errorf("transaction id: %w", err)
	}
	req.Header.Set("accept", "*/*")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-source-app", sourceApp)
	req.Header.Set("x-mobile-app-version", c.AppVersion)
	req.Header.Set("x-transaction-id", txn)
	req.Header.Set("user-agent", userAgent)
	return req, nil
}

// Do performs an id_token-authenticated request. path is joined onto BaseURL; body
// may be nil.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) (*Response, error) {
	if c.IDToken == "" {
		return nil, fmt.Errorf("not authenticated: no id_token (run `safe_cli auth import` or `auth login`)")
	}
	req, err := c.newAppRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	// The app sends the raw id_token as Authorization — no "Bearer " prefix.
	req.Header.Set("Authorization", c.IDToken)
	return c.send(req)
}

// SignedDo performs a SIGNED device-auth request (OTP send/validate, token refresh).
// These endpoints are unauthenticated but require the x-signature header. key is the
// app signing key (apkkey.ResolveKey); appUUID identifies this install (x-appuuid).
func (c *Client) SignedDo(ctx context.Context, method, path string, body []byte, key, appUUID string) (*Response, error) {
	if key == "" {
		return nil, fmt.Errorf("no signing key: set SAFE_CLI_SIGNING_KEY (see `safe_cli auth extract-key`)")
	}
	if appUUID == "" {
		return nil, fmt.Errorf("no app uuid for the signed request")
	}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return nil, err
	}
	// signing.SignedHeaders sets x-transaction-id, x-trace-transaction-id, x-timestamp,
	// x-appuuid, x-signature, x-source-app, and x-mobile-app-version — all consistent
	// with the signed string so the backend's recomputation matches.
	hdrs, err := signing.SignedHeaders(key, method, appUUID, time.Now())
	if err != nil {
		return nil, err
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	req.Header.Set("accept", "*/*")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", userAgent)

	return c.send(req)
}

// CodeExchange carries the inputs for the OAuth authorization-code → token exchange.
// Path is the descriptor's auth.endpoints.user_auth_token; ClientID/RedirectURI come
// from the descriptor (oauth.Settings); Verifier is the PKCE code_verifier.
type CodeExchange struct {
	Path        string
	Code        string
	Verifier    string
	ClientID    string
	RedirectURI string
	AppUUID     string
}

// ExchangeCode trades the captured authorization code (plus the PKCE verifier) for the
// token set. This call is neither id_token-authenticated nor x-signed (§9 step 6, §10),
// so it carries only the mundane app headers. It returns the parsed token set (an
// online set and an offline set; the offline refresh_token is the durable one to keep).
//
// NOTE: the exact request body is confirmed only at the live run; the response shape is
// the captured one and maps directly onto tokenstore.TokenSet.
func (c *Client) ExchangeCode(ctx context.Context, ex CodeExchange) (*tokenstore.TokenSet, error) {
	if ex.Code == "" || ex.Verifier == "" {
		return nil, fmt.Errorf("code exchange needs both the authorization code and the PKCE verifier")
	}
	body, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          ex.Code,
		"code_verifier": ex.Verifier,
		"client_id":     ex.ClientID,
		"redirect_uri":  ex.RedirectURI,
		"app_uuid":      ex.AppUUID,
	})
	if err != nil {
		return nil, err
	}
	req, err := c.newAppRequest(ctx, "POST", ex.Path, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusOK {
		return nil, fmt.Errorf("token exchange: status %d: %s", resp.Status, resp.Body)
	}
	var ts tokenstore.TokenSet
	if err := json.Unmarshal(resp.Body, &ts); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if refreshToken(&ts) == "" {
		return nil, fmt.Errorf("token exchange returned no refresh_token")
	}
	return &ts, nil
}

// refreshToken returns the first refresh_token in the set (the offline set carries the
// durable one).
func refreshToken(ts *tokenstore.TokenSet) string {
	for _, t := range ts.Tokens {
		if t.RefreshToken != "" {
			return t.RefreshToken
		}
	}
	return ""
}

// send executes req and reads the full response body.
func (c *Client) send(req *http.Request) (*Response, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Body: b}, nil
}
