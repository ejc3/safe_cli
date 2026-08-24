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
// NOTE: the parental-control operations are plain id_token calls (via DoH) that also
// carry the x-fp-identifier-* request-identity headers (the target child's service id,
// etc.). They are NOT AWS SigV4/Cognito — that premise was disproven by an empty memory
// scan and the decompiled TokenAwareInterceptor (docs/PROCESS.md §12).
package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ejc3/safe_cli/internal/signing"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// traceID returns a random v4 UUID for the x-trace-transaction-id header the token
// endpoint requires.
func traceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

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
	return c.DoH(ctx, method, path, body, nil)
}

// DoH is Do with additional request headers — the x-fp-identifier-* request-identity
// headers the parental-control endpoints require alongside the id_token.
func (c *Client) DoH(ctx context.Context, method, path string, body []byte, headers map[string]string) (*Response, error) {
	if c.IDToken == "" {
		return nil, fmt.Errorf("not authenticated: no id_token (run `safe_cli auth import` or `auth login`)")
	}
	req, err := c.newAppRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	// The app sends the raw id_token as Authorization — no "Bearer " prefix.
	req.Header.Set("Authorization", c.IDToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
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

// Auth grant/provider constants for the /v7/user/auth/token endpoint. Confirmed live
// (docs/PROCESS.md §9) and against the decompiled ParentTokenRequest model: both the
// authorization-code exchange and the refresh POST this one endpoint with a camelCase
// body carrying only the fields each grant needs.
const (
	identityProviderVZAM = "vz-am-provider"
	grantAuthCode        = "authorization_code"
	grantRefresh         = "refresh_token"
	friscoOnline         = "online"
)

// parentTokenRequest is the app's unified token-endpoint body — the decompiled
// com.verizon.network.model.login.TokenRequest$ParentTokenRequest with its null fields
// dropped. Every field is omitempty so each grant serializes only its own subset.
type parentTokenRequest struct {
	AppUUID          string `json:"appUuid,omitempty"`
	IdentityProvider string `json:"identityProvider,omitempty"`
	GrantType        string `json:"grantType,omitempty"`
	Code             string `json:"code,omitempty"`
	CodeVerifier     string `json:"codeVerifier,omitempty"`
	RefreshToken     string `json:"refreshToken,omitempty"`
	FriscoTokenType  string `json:"friscoTokenType,omitempty"`
	RedirectURI      string `json:"redirectUri,omitempty"`
	ClientID         string `json:"clientId,omitempty"`
	Token            string `json:"token,omitempty"`
}

// postTokenRequest posts a parentTokenRequest to the token endpoint. The call is neither
// id_token-authenticated nor x-signed (§9 step 6, §10) — just the mundane app headers —
// and returns the parsed token set.
func (c *Client) postTokenRequest(ctx context.Context, path string, pr parentTokenRequest) (*tokenstore.TokenSet, error) {
	body, err := json.Marshal(pr)
	if err != nil {
		return nil, err
	}
	req, err := c.newAppRequest(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}
	// The token endpoint requires a trace id: without x-trace-transaction-id it rejects
	// the refresh with 400 "Invalid Request" (confirmed live). The app supplies it from
	// HeaderProvider.getAuthTokenHeaders; a fresh v4 UUID satisfies it.
	trace, err := traceID()
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-trace-transaction-id", trace)
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.Status, resp.Body)
	}
	var ts tokenstore.TokenSet
	if err := json.Unmarshal(resp.Body, &ts); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &ts, nil
}

// CodeExchange carries the inputs for the OAuth authorization-code → token exchange.
// Path is the descriptor's auth.endpoints.user_auth_token; ClientID/RedirectURI come
// from the descriptor (oauth.Settings); Verifier is the PKCE code_verifier; Token is
// the loginRecommendation JWT from the device-OTP validate step — it fuses the device
// factor to the web factor and the backend rejects the exchange without it.
type CodeExchange struct {
	Path             string
	Code             string
	Verifier         string
	ClientID         string
	RedirectURI      string
	AppUUID          string
	Token            string // loginRecommendation JWT (required)
	IdentityProvider string // default vz-am-provider
	FriscoTokenType  string // default online
}

// ExchangeCode trades the captured authorization code (plus the PKCE verifier and the
// device-OTP loginRecommendation token) for the token set. Confirmed live (§9): the body
// is camelCase and the recom token binds factor-1 (device OTP) to factor-2 (web login).
// It returns an online set and an offline set; the offline refresh_token is the durable
// one to keep.
func (c *Client) ExchangeCode(ctx context.Context, ex CodeExchange) (*tokenstore.TokenSet, error) {
	if ex.Code == "" || ex.Verifier == "" {
		return nil, fmt.Errorf("code exchange needs both the authorization code and the PKCE verifier")
	}
	if ex.Token == "" {
		return nil, fmt.Errorf("code exchange needs the loginRecommendation token from the device-OTP validate step")
	}
	idp := ex.IdentityProvider
	if idp == "" {
		idp = identityProviderVZAM
	}
	ftt := ex.FriscoTokenType
	if ftt == "" {
		ftt = friscoOnline
	}
	ts, err := c.postTokenRequest(ctx, ex.Path, parentTokenRequest{
		AppUUID:          ex.AppUUID,
		IdentityProvider: idp,
		GrantType:        grantAuthCode,
		Code:             ex.Code,
		CodeVerifier:     ex.Verifier,
		FriscoTokenType:  ftt,
		RedirectURI:      ex.RedirectURI,
		ClientID:         ex.ClientID,
		Token:            ex.Token,
	})
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	// The durable login persists the OFFLINE (24h) refresh_token; an online-only set is
	// not usable for it, so require the offline entry to carry a refresh_token.
	if off, ok := ts.Offline(); !ok || off.RefreshToken == "" {
		return nil, fmt.Errorf("token exchange returned no offline refresh_token (needed for durable login)")
	}
	return ts, nil
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
