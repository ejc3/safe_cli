package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// RefreshRequest carries the inputs for a device-auth token refresh — a SIGNED call
// (docs/PROCESS.md §10) that trades the offline refresh_token for a fresh token set.
// The exact body is confirmed at the live run; the response maps to tokenstore.TokenSet.
type RefreshRequest struct {
	Path         string // descriptor auth.endpoints.refresh
	RefreshToken string
	ClientID     string
	AppUUID      string
	Key          string // app signing key (this call is signed)
	FriscoType   string // "offline" by default
}

// Refresh performs a device-auth token refresh and returns the new token set.
func (c *Client) Refresh(ctx context.Context, req RefreshRequest) (*tokenstore.TokenSet, error) {
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("refresh needs a refresh_token")
	}
	ft := req.FriscoType
	if ft == "" {
		ft = "offline"
	}
	body, err := json.Marshal(map[string]string{
		"grant_type":        "refresh_token",
		"code":              req.RefreshToken,
		"client_id":         req.ClientID,
		"app_uuid":          req.AppUUID,
		"frisco_token_type": ft,
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.SignedDo(ctx, "POST", req.Path, body, req.Key, req.AppUUID)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusOK {
		return nil, fmt.Errorf("refresh: status %d: %s", resp.Status, resp.Body)
	}
	var ts tokenstore.TokenSet
	if err := json.Unmarshal(resp.Body, &ts); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}
	if _, ok := ts.IDToken(); !ok {
		return nil, fmt.Errorf("refresh returned no id_token")
	}
	return &ts, nil
}
