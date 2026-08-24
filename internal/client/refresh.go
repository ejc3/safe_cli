package client

import (
	"context"
	"fmt"

	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// RefreshRequest carries the inputs for a token refresh. Confirmed live (docs/PROCESS.md
// §10): the refresh is NOT signed and hits the same /v7/user/auth/token endpoint as the
// code exchange (Path is auth.endpoints.user_auth_token). FriscoType MUST match the
// refresh token's own type — an online refresh_token with "online", an offline one with
// "offline"; the backend answers a mismatch with 400 "Invalid Request". An online refresh
// returns a fresh online+offline set; an offline refresh returns only a new offline token.
type RefreshRequest struct {
	Path             string
	RefreshToken     string
	ClientID         string
	AppUUID          string
	RedirectURI      string
	FriscoType       string // must match the refresh token's type ("online"/"offline")
	IdentityProvider string // default vz-am-provider
}

// Refresh performs a token refresh and returns the new token set.
func (c *Client) Refresh(ctx context.Context, req RefreshRequest) (*tokenstore.TokenSet, error) {
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("refresh needs a refresh_token")
	}
	ft := req.FriscoType
	if ft == "" {
		ft = friscoOnline
	}
	idp := req.IdentityProvider
	if idp == "" {
		idp = identityProviderVZAM
	}
	ts, err := c.postTokenRequest(ctx, req.Path, parentTokenRequest{
		AppUUID:          req.AppUUID,
		IdentityProvider: idp,
		GrantType:        grantRefresh,
		RefreshToken:     req.RefreshToken,
		FriscoTokenType:  ft,
		RedirectURI:      req.RedirectURI,
		ClientID:         req.ClientID,
	})
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	if _, ok := ts.IDToken(); !ok {
		return nil, fmt.Errorf("refresh returned no id_token")
	}
	return ts, nil
}
