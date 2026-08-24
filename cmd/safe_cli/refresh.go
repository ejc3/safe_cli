package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/deviceid"
	"github.com/ejc3/safe_cli/internal/outfmt"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// authRefreshCmd refreshes the id_token using the stored offline refresh_token — the
// signed device-auth refresh (docs/PROCESS.md §10). No OTP or browser needed.
type authRefreshCmd struct {
	APK string `name:"apk" type:"existingfile" help:"Extract the signing key from this APK instead of SAFE_CLI_SIGNING_KEY[_FILE]."`
}

func (c *authRefreshCmd) Run(rc *runContext) error {
	st, old, err := loadTokens()
	if err != nil {
		return err
	}
	key, err := signingKey(c.APK)
	if err != nil {
		return err
	}
	appUUID := old.AppUUID
	if appUUID == "" {
		if appUUID, err = deviceid.AppUUID(); err != nil {
			return fmt.Errorf("app uuid: %w", err)
		}
	}
	cl := client.New("")
	if rc.D.BaseURL != "" {
		cl.BaseURL = rc.D.BaseURL
	}
	ts, err := refreshTokens(context.Background(), cl, rc.D.Auth.Endpoints["refresh"], rc.D.Auth.ClientID, old, key, appUUID)
	if err != nil {
		return err
	}
	if err := st.Save(ts, time.Now()); err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}
	return writeRefreshResult(rc.Out, rc.G.JSON, ts)
}

// refreshTokens performs the refresh against cl and returns the new set, carrying over
// identity fields and preserving the offline refresh_token if the response omits one.
func refreshTokens(ctx context.Context, cl *client.Client, refreshPath, clientID string, old *tokenstore.TokenSet, key, appUUID string) (*tokenstore.TokenSet, error) {
	off, ok := old.Offline()
	if !ok || off.RefreshToken == "" {
		return nil, fmt.Errorf("no offline refresh_token in the stored tokens; run `safe_cli auth login`")
	}
	ts, err := cl.Refresh(ctx, client.RefreshRequest{
		Path:         refreshPath,
		RefreshToken: off.RefreshToken,
		ClientID:     clientID,
		AppUUID:      appUUID,
		Key:          key,
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
	// Keep a durable credential for the next refresh: if the response has no offline
	// entry, carry the old one wholesale; if it has one but WITHOUT a refresh_token
	// (e.g. the existing credential stays valid), copy the old refresh_token into it.
	if newOff, ok := ts.Offline(); !ok {
		ts.Tokens = append(ts.Tokens, off)
	} else if newOff.RefreshToken == "" {
		for i := range ts.Tokens {
			if ts.Tokens[i].FriscoTokenType == "offline" {
				ts.Tokens[i].RefreshToken = off.RefreshToken
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
