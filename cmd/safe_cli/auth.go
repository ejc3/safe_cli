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
	Login      authLoginCmd      `cmd:"" help:"One-time assisted login: device OTP, hosted Verizon login + 2FA, token exchange."`
	Import     authImportCmd     `cmd:"" help:"Import a captured frisco token JSON and persist it (0600)."`
	Status     authStatusCmd     `cmd:"" help:"Show stored token status."`
	Logout     authLogoutCmd     `cmd:"" help:"Delete stored tokens."`
	ExtractKey authExtractKeyCmd `cmd:"" name:"extract-key" help:"Read the request-signing key from your own APK and print it."`
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

type authImportCmd struct {
	File string `arg:"" type:"existingfile" help:"Path to the frisco token JSON (the token endpoint response)."`
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
		if _, err := fmt.Fprintf(rc.Out, "mdn: %s\n", ts.MDN); err != nil {
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
