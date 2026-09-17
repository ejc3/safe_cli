package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ejc3/safe_cli/internal/apkkey"
	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/outfmt"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// extractSigningKey is a package var so tests can stub the APK read.
var extractSigningKey = apkkey.ExtractSigningKey

// redact keeps a token recognizable without exposing it.
func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 12 {
		return "…"
	}
	return s[:8] + "…(" + fmt.Sprintf("%d", len(s)) + " chars)"
}

func loadTokens() (*tokenstore.Store, *tokenstore.TokenSet, error) {
	st, err := tokenstore.DefaultStore()
	if err != nil {
		return nil, nil, err
	}
	ts, err := st.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil, fmt.Errorf("not authenticated: no token file at %s (run `safe_cli auth import`)", st.Path)
		}
		return st, nil, err
	}
	return st, ts, nil
}

type authCmd struct {
	Login          authLoginCmd          `cmd:"" help:"One-time assisted login: device OTP, hosted Verizon login + 2FA, token exchange."`
	Refresh        authRefreshCmd        `cmd:"" help:"Refresh the id_token using the stored online refresh_token (no OTP)."`
	Import         authImportCmd         `cmd:"" help:"Import a token bundle (from 'auth export' or a captured frisco token JSON) and persist it (0600)."`
	Export         authExportCmd         `cmd:"" help:"Export the stored token bundle so another device can authenticate without repeating the browser login; import it there with 'auth import'."`
	Status         authStatusCmd         `cmd:"" help:"Show stored token status."`
	Logout         authLogoutCmd         `cmd:"" help:"Delete stored tokens."`
	ExtractKey     authExtractKeyCmd     `cmd:"" name:"extract-key" help:"Read the request-signing key from your own APK and print it."`
	RegisterScheme authRegisterSchemeCmd `cmd:"" name:"register-scheme" help:"macOS: register the vsfapp:// handler so 'auth login' captures the browser redirect automatically."`
}

// authExtractKeyCmd surfaces the app's HMAC request-signing key from the operator's
// own licensed APK. The key is not shipped with this tool; this is how the operator
// supplies it (see docs/PROCESS.md §11). Its output IS the key — pipe or capture it,
// e.g. `export SAFE_CLI_SIGNING_KEY=$(safe_cli auth extract-key --apk your.apk)`.
type authExtractKeyCmd struct {
	APK string `name:"apk" type:"existingfile" required:"" help:"Path to your own Verizon Family APK."`
}

func (c *authExtractKeyCmd) Run(rc *runContext) error {
	key, err := extractSigningKey(c.APK)
	if err != nil {
		return err
	}
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, map[string]string{"signing_key": key})
	}
	_, err = fmt.Fprintln(rc.Out, key)
	return err
}

// authExportCmd writes the stored token bundle so auth can be moved to another device:
// export here, `auth import` there. The bundle carries the durable OFFLINE refresh token
// and the app_uuid, so the other device can `auth refresh` for fresh id_tokens without
// repeating the assisted browser login. It is a SECRET — anyone holding it can mint tokens
// for the account — so write it to a 0600 file or a trusted channel, never a shared path.
type authExportCmd struct {
	File string `name:"file" type:"path" help:"Write the bundle to this file (0600) instead of stdout."`
}

func (c *authExportCmd) Run(rc *runContext) error {
	st, err := tokenstore.DefaultStore()
	if err != nil {
		return err
	}
	ts, err := st.Load()
	if err != nil {
		return err
	}
	if len(ts.Tokens) == 0 {
		return fmt.Errorf("no tokens to export; run 'safe_cli auth login' first")
	}
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if c.File == "" {
		// stdout is the raw bundle so `safe_cli auth export > bundle.json` round-trips into
		// `safe_cli auth import bundle.json`. Progress/warnings go to stderr via the caller.
		_, err = rc.Out.Write(data)
		return err
	}
	if err := os.WriteFile(c.File, data, 0o600); err != nil {
		return err
	}
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, map[string]any{"exported": len(ts.Tokens), "path": c.File})
	}
	_, err = fmt.Fprintf(rc.Out, "exported %d token(s) -> %s (0600).\n"+
		"On another device: safe_cli auth import %s  (then `safe_cli auth refresh`).\n"+
		"This file is a SECRET — it can mint tokens for the account; delete it after import.\n",
		len(ts.Tokens), c.File, c.File)
	return err
}

type authImportCmd struct {
	File string `arg:"" type:"existingfile" help:"Path to a token bundle from 'auth export', or a captured frisco token JSON."`
}

func (c *authImportCmd) Run(rc *runContext) error {
	b, err := os.ReadFile(c.File)
	if err != nil {
		return err
	}
	var ts tokenstore.TokenSet
	if err := json.Unmarshal(b, &ts); err != nil {
		return fmt.Errorf("parse token json: %w", err)
	}
	if len(ts.Tokens) == 0 {
		return fmt.Errorf("no tokens found in %s", c.File)
	}
	st, err := tokenstore.DefaultStore()
	if err != nil {
		return err
	}
	if err := st.Save(&ts, time.Now()); err != nil {
		return err
	}
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, map[string]any{"imported": len(ts.Tokens), "path": st.Path})
	}
	_, err = fmt.Fprintf(rc.Out, "imported %d token(s) -> %s\n", len(ts.Tokens), st.Path)
	return err
}

type authStatusCmd struct{}

func (c *authStatusCmd) Run(rc *runContext) error {
	_, ts, err := loadTokens()
	if err != nil {
		return err
	}
	if rc.G.JSON {
		red := *ts
		red.Tokens = make([]tokenstore.Token, len(ts.Tokens))
		copy(red.Tokens, ts.Tokens)
		for i := range red.Tokens {
			red.Tokens[i].IDToken = redact(red.Tokens[i].IDToken)
			red.Tokens[i].AccessToken = redact(red.Tokens[i].AccessToken)
			red.Tokens[i].RefreshToken = redact(red.Tokens[i].RefreshToken)
		}
		return outfmt.JSON(rc.Out, red)
	}
	var rows [][]string
	for _, t := range ts.Tokens {
		state := "valid"
		if t.Expired(0) {
			state = "EXPIRED"
		}
		rows = append(rows, []string{t.FriscoTokenType, fmt.Sprintf("%ds", t.ExpiresIn), state, redact(t.IDToken)})
	}
	if ts.MDN != "" {
		if _, err := fmt.Fprintf(rc.Out, "phone: %s\n", ts.MDN); err != nil {
			return err
		}
	}
	return outfmt.Table(rc.Out, []string{"TYPE", "EXPIRES_IN", "STATE", "ID_TOKEN"}, rows)
}

type authLogoutCmd struct{}

func (c *authLogoutCmd) Run(rc *runContext) error {
	st, err := tokenstore.DefaultStore()
	if err != nil {
		return err
	}
	if err := st.Delete(); err != nil {
		return err
	}
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, map[string]any{"loggedOut": true, "path": st.Path})
	}
	_, err = fmt.Fprintf(rc.Out, "logged out (removed %s)\n", st.Path)
	return err
}

type rawCmd struct {
	Method string `arg:"" enum:"GET,POST,PUT,PATCH,DELETE,get,post,put,patch,delete" help:"HTTP method."`
	Path   string `arg:"" help:"Backend path, e.g. /auth/frisco/mappcontent/v6/configs"`
	Body   string `short:"d" help:"Optional JSON request body."`
}

func (c *rawCmd) Run(rc *runContext) error {
	_, ts, err := loadTokens()
	if err != nil {
		return err
	}
	idt, ok := ts.IDToken()
	if !ok {
		return fmt.Errorf("stored tokens have no id_token")
	}
	cl := client.New(idt)
	var body []byte
	if c.Body != "" {
		body = []byte(c.Body)
	}
	resp, err := cl.Do(context.Background(), strings.ToUpper(c.Method), c.Path, body)
	if err != nil {
		return err
	}
	// Status to stderr so stdout stays the response body (stdout-as-API).
	_, _ = fmt.Fprintf(os.Stderr, "HTTP %d\n", resp.Status)
	if _, err := rc.Out.Write(resp.Body); err != nil {
		return err
	}
	if n := len(resp.Body); n == 0 || resp.Body[n-1] != '\n' {
		_, _ = fmt.Fprintln(rc.Out)
	}
	if resp.Status >= 400 {
		return fmt.Errorf("request failed: HTTP %d", resp.Status)
	}
	return nil
}
