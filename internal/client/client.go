// Package client is the SafePath (Verizon Family) HTTP client. It sends the
// id_token as the Authorization header (raw JWT, no "Bearer " prefix — that is how
// the app authenticates config/content endpoints) plus the app's mundane headers.
//
// NOTE: the parental-control operations use AWS SigV4 (Cognito temp creds), not the
// id_token — see docs/PROCESS.md §10. Those are handled by a separate signer added
// in a later phase; this client covers the id_token-authed surface and `raw`.
package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
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

// txnID returns a 20-digit numeric transaction id like the app sends.
func txnID() string {
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// crypto/rand should not fail; degrade to a fixed digit rather than panic.
			sb.WriteByte('0')
			continue
		}
		sb.WriteByte(byte('0' + n.Int64()))
	}
	return sb.String()
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
	req.Header.Set("x-transaction-id", txnID())
	req.Header.Set("user-agent", userAgent)

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
