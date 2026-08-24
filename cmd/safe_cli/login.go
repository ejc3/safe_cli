package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ejc3/safe_cli/internal/apkkey"
	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/deviceid"
	"github.com/ejc3/safe_cli/internal/oauth"
	"github.com/ejc3/safe_cli/internal/outfmt"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// authLoginCmd runs the one-time assisted login (docs/PROCESS.md §9): the signed
// device-OTP legs the CLI owns directly, then a real browser for the Akamai-gated
// My-Verizon web login + 2FA, whose vsfapp:// redirect the operator pastes back, then
// the token exchange. The durable offline refresh_token is persisted.
type authLoginCmd struct {
	MDN       string `name:"mdn" help:"The 10-digit line to verify (prompted if omitted)."`
	APK       string `name:"apk" type:"existingfile" help:"Extract the signing key from this APK instead of SAFE_CLI_SIGNING_KEY[_FILE]."`
	Redirect  string `name:"redirect" help:"Override the OAuth redirect_uri (advanced; e.g. an RFC 8252 loopback)."`
	NoBrowser bool   `name:"no-browser" help:"Do not try to open a browser; just print the URL."`
	Paste     bool   `name:"paste" help:"Paste the vsfapp:// redirect manually instead of the macOS scheme handler."`
}

func (c *authLoginCmd) Run(rc *runContext) error {
	key, err := signingKey(c.APK)
	if err != nil {
		return err
	}
	appUUID, err := deviceid.AppUUID()
	if err != nil {
		return fmt.Errorf("app uuid: %w", err)
	}
	cl := client.New("")
	if rc.D.BaseURL != "" {
		cl.BaseURL = rc.D.BaseURL
	}
	open := openBrowser
	if c.NoBrowser {
		open = nil
	}
	deps := loginDeps{
		Client:   cl,
		Desc:     rc.D,
		Key:      key,
		AppUUID:  appUUID,
		MDN:      c.MDN,
		Redirect: c.Redirect,
		In:       bufio.NewReader(os.Stdin),
		Out:      os.Stderr, // interactive prompts/messages go to stderr; stdout stays the API
		OpenURL:  open,
		Now:      time.Now(),
	}
	// On macOS, register the vsfapp:// handler and capture the redirect automatically —
	// unless --paste is set or --redirect points somewhere other than the vsfapp scheme.
	if !c.Paste && runtime.GOOS == "darwin" && (c.Redirect == "" || strings.HasPrefix(c.Redirect, vsfappScheme+"://")) {
		// If the handler can't be registered (locked-down Mac, MDM, missing dev tools),
		// fall back to manual paste (WaitRedirect stays nil) rather than aborting login.
		if _, redirectFile, err := registerScheme(os.Stderr); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "(couldn't register the vsfapp:// handler: %v — falling back to manual paste)\n", err)
		} else {
			_ = os.Remove(redirectFile) // drop any stale capture before we start
			deps.WaitRedirect = func(ctx context.Context) (string, error) {
				wctx, cancel := context.WithTimeout(ctx, defaultRedirectTo)
				defer cancel()
				return waitForRedirect(wctx, redirectFile, time.Second)
			}
		}
	}
	ts, err := runLogin(context.Background(), deps)
	if err != nil {
		return err
	}
	st, err := tokenstore.DefaultStore()
	if err != nil {
		return err
	}
	if err := st.Save(ts, time.Now()); err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}
	return writeLoginResult(rc.Out, rc.G.JSON, ts.MDN, st.Path)
}

// writeLoginResult emits the final result to stdout: a JSON object under --json, else a
// human line. Interactive prompts/progress went to stderr, so stdout stays clean.
func writeLoginResult(out io.Writer, asJSON bool, mdn, path string) error {
	if asJSON {
		return outfmt.JSON(out, map[string]string{
			"status":      "ok",
			"mdn":         mdn,
			"tokens_path": path,
		})
	}
	_, err := fmt.Fprintf(out, "Logged in as %s. Tokens saved to %s.\n", mdn, path)
	return err
}

// signingKey resolves the app signing key: from the given APK if provided, else the
// SAFE_CLI_SIGNING_KEY[_FILE] environment.
func signingKey(apk string) (string, error) {
	if apk != "" {
		return extractSigningKey(apk)
	}
	return apkkey.ResolveKey()
}

// loginDeps are the inputs to the testable login core. The generators default to the
// real crypto sources; tests inject deterministic ones so the pasted redirect's state
// is known ahead of time.
type loginDeps struct {
	Client   *client.Client
	Desc     *descriptor.Descriptor
	Key      string
	AppUUID  string
	MDN      string
	Redirect string
	In       *bufio.Reader
	Out      io.Writer
	OpenURL  func(string) error
	Now      time.Time

	// WaitRedirect, when set, captures the browser's vsfapp:// redirect automatically
	// (the macOS scheme handler) instead of prompting the operator to paste it.
	WaitRedirect func(context.Context) (string, error)

	genPKCE func() (oauth.PKCE, error)
}

func runLogin(ctx context.Context, d loginDeps) (*tokenstore.TokenSet, error) {
	if d.genPKCE == nil {
		d.genPKCE = oauth.GeneratePKCE
	}
	ep := d.Desc.Auth.Endpoints
	settings := oauth.FromDescriptor(d.Desc.Auth)
	redirect := d.Redirect
	if redirect == "" {
		redirect = settings.RedirectURI
	}

	// Factor 1: SafePath device OTP (signed frisco call).
	mdn := strings.TrimSpace(d.MDN)
	if mdn == "" {
		var err error
		if mdn, err = promptLine(d.In, d.Out, "Verizon line (10-digit MDN): "); err != nil {
			return nil, err
		}
	}
	resp, err := d.Client.RequestDeviceOTP(ctx, ep["otp_request"], mdn, d.Key, d.AppUUID)
	if err != nil {
		return nil, fmt.Errorf("request device OTP: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("request device OTP: status %d: %s", resp.Status, resp.Body)
	}
	_, _ = fmt.Fprintf(d.Out, "An SMS code was sent to %s.\n", mdn)
	otp, err := promptLine(d.In, d.Out, "Device OTP code: ")
	if err != nil {
		return nil, err
	}
	dev, err := d.Client.ValidateDeviceOTP(ctx, ep["otp_validate"], mdn, otp, d.Key, d.AppUUID)
	if err != nil {
		return nil, fmt.Errorf("validate device OTP: %w", err)
	}

	// Assisted hosted login: build the authorize URL and let a real browser complete
	// the Akamai-gated My-Verizon login + 2FA; capture the vsfapp:// redirect.
	pkce, err := d.genPKCE()
	if err != nil {
		return nil, err
	}
	authURL, _, err := settings.AuthorizeURL(d.Desc.BaseURL, redirect, d.AppUUID, pkce.Challenge)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(d.Out, "\nOpen this URL and complete the Verizon account login and 2FA:\n\n  %s\n\n", authURL)
	if d.OpenURL != nil {
		if err := d.OpenURL(authURL); err != nil {
			_, _ = fmt.Fprintf(d.Out, "(couldn't open a browser automatically: %v — open the URL above manually)\n", err)
		}
	}

	// Capture the vsfapp://…?code=… redirect: automatically via the OS scheme handler
	// when wired (macOS), else by paste. State is checked leniently — the frisco backend
	// rebinds state per app_uuid, so the returned value need not match ours; the code is
	// worthless without our PKCE verifier and the recom token at exchange.
	var redirected string
	if d.WaitRedirect != nil {
		_, _ = fmt.Fprintf(d.Out, "Waiting for the browser redirect (the vsfapp:// handler will capture it automatically)…\n")
		redirected, err = d.WaitRedirect(ctx)
	} else {
		_, _ = fmt.Fprintf(d.Out, "After signing in, the browser is redirected to a %s… URL it cannot open.\n", redirect)
		redirected, err = promptLine(d.In, d.Out, "Paste that full redirect URL here: ")
	}
	if err != nil {
		return nil, err
	}
	code, err := oauth.ParseRedirect(redirected, redirect, "")
	if err != nil {
		return nil, err
	}

	// Exchange the code (with the PKCE verifier we own) for the token set.
	ts, err := d.Client.ExchangeCode(ctx, client.CodeExchange{
		Path:        ep["user_auth_token"],
		Code:        code,
		Verifier:    pkce.Verifier,
		ClientID:    settings.ClientID,
		RedirectURI: redirect,
		AppUUID:     d.AppUUID,
		Token:       dev.LoginRecomToken,
	})
	if err != nil {
		return nil, err
	}
	ts.MDN = mdn
	ts.AppUUID = d.AppUUID
	return ts, nil
}

func promptLine(in *bufio.Reader, out io.Writer, label string) (string, error) {
	_, _ = fmt.Fprint(out, label)
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("no input for %q", strings.TrimSpace(label))
	}
	return line, nil
}

// browserCommand returns the opener for goos and the argv to pass url. It avoids
// cmd.exe on Windows: the authorize URL's `&` separators would be treated as command
// separators by `cmd /c start`, truncating the URL. rundll32's FileProtocolHandler
// takes the URL as a single argv element with no shell interpretation.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// openBrowser best-effort opens url in the operator's default browser.
func openBrowser(url string) error {
	name, args := browserCommand(runtime.GOOS, url)
	// Background context: this is a fire-and-forget Start(); we do not want to kill the
	// browser when the login context ends.
	// #nosec G204 -- name is a fixed value chosen by GOOS; args are argv (no shell), so
	// the authorize URL cannot be interpreted as a command.
	return exec.CommandContext(context.Background(), name, args...).Start()
}
