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
//
// NOTE: the parental-control operations use AWS SigV4 (Cognito temp creds), not the
// id_token — see docs/PROCESS.md §10. Those are handled by a separate signer added
// in a later phase.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ejc3/safe_cli/internal/signing"
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

// Do performs a request. path is joined onto BaseURL; body may be nil.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) (*Response, error) {
	if c.IDToken == "" {
		return nil, fmt.Errorf("not authenticated: no id_token (run `safe_cli auth import` or `auth login`)")
	}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return nil, err
	}
	// The app sends the raw id_token as Authorization — no "Bearer " prefix.
	req.Header.Set("Authorization", c.IDToken)
	req.Header.Set("accept", "*/*")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-source-app", sourceApp)
	req.Header.Set("x-mobile-app-version", c.AppVersion)
	txn, err := signing.TransactionID()
	if err != nil {
		return nil, fmt.Errorf("transaction id: %w", err)
	}
	req.Header.Set("x-transaction-id", txn)
	req.Header.Set("user-agent", userAgent)

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
