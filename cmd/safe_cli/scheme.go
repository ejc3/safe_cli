package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The custom URI scheme the frisco OAuth redirect_uri uses. On macOS we register a tiny
// handler app for it so the browser's `vsfapp://…/signin?code=…` redirect is delivered
// straight to a waiting `auth login` — no copy/paste, no cloud browser.
const (
	vsfappScheme      = "vsfapp"
	handlerAppName    = "SafeCLIVsfappHandler.app"
	handlerBundleID   = "com.ejc3.safe-cli.vsfapp-handler"
	handlerURLName    = "com.ejc3.safe-cli.vsfapp"
	redirectFileName  = "vsfapp_redirect"
	lsregister        = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	osacompile        = "/usr/bin/osacompile"
	plistBuddy        = "/usr/libexec/PlistBuddy"
	defaultRedirectTo = 10 * time.Minute
)

// vsfappRedirectPath is the file the macOS handler writes the captured redirect URL to and
// a waiting `auth login` reads it from. It sits beside the config so both agree on it.
func vsfappRedirectPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "safe_cli", redirectFileName), nil
}

// appletSource returns the AppleScript for the handler applet. AppleScript's
// `on open location` is the no-cgo way to receive a URL-scheme open event; the handler
// writes the URL to redirectFile (absolute path baked in at registration time) via a
// shell one-liner, with `quoted form of` doing the shell-quoting.
func appletSource(redirectFile string) string {
	esc := func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return s
	}
	return fmt.Sprintf(`on open location this_URL
    do shell script "/bin/mkdir -p " & quoted form of "%s" & " && /usr/bin/printf '%%s' " & quoted form of this_URL & " > " & quoted form of "%s"
end open location
`, esc(filepath.Dir(redirectFile)), esc(redirectFile))
}

// plistBuddyCommands returns the PlistBuddy edits that turn the bare osacompile applet into
// a URL-scheme handler for vsfapp:// (a background agent, no dock icon). Each scalar key is
// deleted first so the following Add is deterministic whether or not the osacompile version
// pre-populated it; the leading "Delete " commands are allowed to fail (see registerScheme).
func plistBuddyCommands() []string {
	return []string{
		"Delete :CFBundleURLTypes",
		"Add :CFBundleURLTypes array",
		"Add :CFBundleURLTypes:0 dict",
		"Add :CFBundleURLTypes:0:CFBundleURLName string " + handlerURLName,
		"Add :CFBundleURLTypes:0:CFBundleURLSchemes array",
		"Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string " + vsfappScheme,
		"Delete :CFBundleIdentifier",
		"Add :CFBundleIdentifier string " + handlerBundleID,
		"Delete :LSUIElement",
		"Add :LSUIElement bool true",
	}
}

// schemeGOOS is runtime.GOOS, overridable in tests to exercise the platform guard.
var schemeGOOS = runtime.GOOS

// registerScheme builds and registers the macOS handler app for vsfapp:// and returns the
// installed app path and the redirect file the handler writes to. It is idempotent: it
// rebuilds the app from scratch each call. macOS only.
func registerScheme(w io.Writer) (appPath, redirectFile string, err error) {
	if schemeGOOS != "darwin" {
		return "", "", fmt.Errorf("vsfapp:// scheme registration is macOS-only (this is %s); on other platforms complete the login with --paste", schemeGOOS)
	}
	redirectFile, err = vsfappRedirectPath()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(redirectFile), 0o755); err != nil {
		return "", "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	appsDir := filepath.Join(home, "Applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return "", "", err
	}
	appPath = filepath.Join(appsDir, handlerAppName)
	_ = os.RemoveAll(appPath)

	scriptFile, err := os.CreateTemp("", "safecli-vsfapp-*.applescript")
	if err != nil {
		return "", "", err
	}
	defer func() { _ = os.Remove(scriptFile.Name()) }()
	if _, err := scriptFile.WriteString(appletSource(redirectFile)); err != nil {
		_ = scriptFile.Close()
		return "", "", err
	}
	if err := scriptFile.Close(); err != nil {
		return "", "", err
	}

	if err := runTool(osacompile, "-o", appPath, scriptFile.Name()); err != nil {
		return "", "", fmt.Errorf("osacompile: %w", err)
	}
	plist := filepath.Join(appPath, "Contents", "Info.plist")
	for _, c := range plistBuddyCommands() {
		// "Delete" clears a key that may not exist; tolerate its failure. Every "Add"
		// must succeed.
		if err := runTool(plistBuddy, "-c", c, plist); err != nil && !strings.HasPrefix(c, "Delete ") {
			return "", "", fmt.Errorf("PlistBuddy %q: %w", c, err)
		}
	}
	if err := runTool(lsregister, "-f", appPath); err != nil {
		return "", "", fmt.Errorf("lsregister: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Registered vsfapp:// handler: %s\n", appPath)
	return appPath, redirectFile, nil
}

// runTool runs a fixed system tool and folds its output into any error.
func runTool(name string, args ...string) error {
	// #nosec G204 -- name is a fixed constant path; args are literal tool flags.
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", filepath.Base(name), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// waitForRedirect polls redirectFile until the handler drops the captured vsfapp:// URL,
// or ctx is done. It returns the trimmed URL.
func waitForRedirect(ctx context.Context, redirectFile string, poll time.Duration) (string, error) {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		if b, err := os.ReadFile(redirectFile); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-t.C:
		}
	}
}

// authRegisterSchemeCmd installs the macOS vsfapp:// handler so `auth login` can capture
// the browser redirect automatically.
type authRegisterSchemeCmd struct{}

func (c *authRegisterSchemeCmd) Run(rc *runContext) error {
	appPath, redirectFile, err := registerScheme(rc.Out)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(rc.Out, "Redirect capture file: %s\n", redirectFile)
	_, _ = fmt.Fprintf(rc.Out, "Test it:\n  open %q\n  cat %q\n",
		vsfappScheme+"://com.verizon.familybase.parent/signin?code=TESTCODE&state=x", redirectFile)
	_ = appPath
	return nil
}
