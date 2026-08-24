package main

import (
	"context"
	"net/http"
	"time"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// authedRequest returns a doFunc that sends an id_token-authenticated request with the
// given extra request-identity headers and, on a 401 (expired id_token), transparently
// refreshes the token once and retries. It takes the already-loaded store and token set
// (callCmd.Run loads them once for the identity headers), so it is the production path;
// tests inject a plain client instead.
func authedRequest(rc *runContext, st *tokenstore.Store, ts *tokenstore.TokenSet) doFunc {
	idt, _ := ts.IDToken()
	cl := client.New(idt)
	if rc.D.BaseURL != "" {
		cl.BaseURL = rc.D.BaseURL
	}
	return func(ctx context.Context, method, path string, body []byte, headers map[string]string) (*client.Response, error) {
		resp, err := cl.DoH(ctx, method, path, body, headers)
		if err != nil {
			return nil, err
		}
		if resp.Status != http.StatusUnauthorized {
			return resp, nil
		}
		// Expired id_token: refresh once and retry. If the refresh can't produce a fresh
		// token for any reason, surface the original 401 rather than masking it.
		nidt := refreshedIDToken(ctx, rc, st, ts)
		if nidt == "" {
			return resp, nil
		}
		cl.IDToken = nidt
		return cl.DoH(ctx, method, path, body, headers)
	}
}

// refreshedIDToken attempts a single best-effort token refresh and returns the new
// id_token, or "" if any step fails (the caller then keeps the original 401). The refreshed
// set is persisted best-effort: a save failure (read-only FS, disk full) must not discard
// the fresh in-memory token that can still serve the retry.
func refreshedIDToken(ctx context.Context, rc *runContext, st *tokenstore.Store, ts *tokenstore.TokenSet) string {
	appUUID, err := resolveAppUUID(ts)
	if err != nil {
		return ""
	}
	newTs, err := refreshOnce(ctx, rc.D, ts, appUUID)
	if err != nil {
		return ""
	}
	_ = st.Save(newTs, time.Now())
	nidt, _ := newTs.IDToken()
	return nidt
}
