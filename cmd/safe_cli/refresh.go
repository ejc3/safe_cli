package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/deviceid"
	"github.com/ejc3/safe_cli/internal/oauth"
	"github.com/ejc3/safe_cli/internal/outfmt"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// authRefreshCmd renews the id_token using the stored online refresh_token — the
// unsigned token-endpoint refresh (docs/PROCESS.md §10). No OTP, browser, or signing
// key needed; it returns a fresh online+offline set.
type authRefreshCmd struct{}

func (c *authRefreshCmd) Run(rc *runContext) error {
	st, old, err := loadTokens()
	if err != nil {
		return err
	}
	appUUID, err := resolveAppUUID(old)
	if err != nil {
		return fmt.Errorf("app uuid: %w", err)
	}
	ts, err := refreshOnce(context.Background(), rc.D, old, appUUID)
	if err != nil {
		return err
	}
	if err := st.Save(ts, time.Now()); err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}
	return writeRefreshResult(rc.Out, rc.G.JSON, ts)
}

// resolveAppUUID returns this install's app_uuid: the value saved with the tokens if
// present, else the device's persisted value.
func resolveAppUUID(ts *tokenstore.TokenSet) (string, error) {
	if ts != nil && ts.AppUUID != "" {
		return ts.AppUUID, nil
	}
	return deviceid.AppUUID()
}

// refreshOnce renews the token set against the backend using the stored refresh token and
// returns the fresh set WITHOUT persisting it — callers save per their own error policy
// (the interactive command treats a save failure as fatal; the on-401 retry does not).
func refreshOnce(ctx context.Context, d *descriptor.Descriptor, old *tokenstore.TokenSet, appUUID string) (*tokenstore.TokenSet, error) {
	cl := client.New("")
	if d.BaseURL != "" {
		cl.BaseURL = d.BaseURL
	}
	settings := oauth.FromDescriptor(d.Auth)
	return refreshTokens(ctx, cl, d.Auth.Endpoints["user_auth_token"], settings.ClientID, settings.RedirectURI, old, appUUID)
}

// refreshTokens performs the refresh against cl and returns the new set, carrying over
// identity fields. It PREFERS the online refresh_token (friscoType "online"), which
// returns a fresh online+offline pair; if only the durable offline refresh_token is
// present — e.g. an imported offline-only set — it falls back to that (friscoType
// "offline", returning a new offline token) rather than forcing a full re-login. The
// friscoType MUST match the token's own type (the backend 400s a mismatch). If an online
// response omits the offline entry, the old one is carried forward so the durable
// credential survives.
func refreshTokens(ctx context.Context, cl *client.Client, tokenPath, clientID, redirectURI string, old *tokenstore.TokenSet, appUUID string) (*tokenstore.TokenSet, error) {
	var refreshToken, friscoType string
	if on, ok := old.Online(); ok && on.RefreshToken != "" {
		refreshToken, friscoType = on.RefreshToken, "online"
	} else if off, ok := old.Offline(); ok && off.RefreshToken != "" {
		refreshToken, friscoType = off.RefreshToken, "offline"
	} else {
		return nil, fmt.Errorf("no online or offline refresh_token in the stored tokens; run `safe_cli auth login`")
	}
	ts, err := cl.Refresh(ctx, client.RefreshRequest{
		Path:         tokenPath,
		RefreshToken: refreshToken,
		ClientID:     clientID,
		AppUUID:      appUUID,
		RedirectURI:  redirectURI,
		FriscoType:   friscoType,
	})
	if err != nil {
		return nil, err
	}
	if ts.MDN == "" {
		ts.MDN = old.MDN
	}
	if ts.AppUUID == "" {
		ts.AppUUID = old.AppUUID
	}
	// An online refresh returns a fresh offline set too. Defend the durable credential:
	// if the response omits the offline entry, carry the old one wholesale; if it has an
	// offline entry WITHOUT a refresh_token, copy the old refresh_token into it.
	if newOff, ok := ts.Offline(); !ok {
		if off, ok := old.Offline(); ok {
			ts.Tokens = append(ts.Tokens, off)
		}
	} else if newOff.RefreshToken == "" {
		if off, ok := old.Offline(); ok {
			for i := range ts.Tokens {
				if ts.Tokens[i].FriscoTokenType == "offline" {
					ts.Tokens[i].RefreshToken = off.RefreshToken
				}
			}
		}
	}
	return ts, nil
}

func writeRefreshResult(out io.Writer, asJSON bool, ts *tokenstore.TokenSet) error {
	if asJSON {
		return outfmt.JSON(out, map[string]string{"status": "ok", "mdn": ts.MDN})
	}
	_, err := fmt.Fprintln(out, "Tokens refreshed.")
	return err
}
