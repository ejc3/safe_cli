# safe_cli

**Verizon Family (Smith Micro SafePath) in your terminal.** A comprehensive,
`gog`/GAM-style CLI that exposes the entire SafePath data model and every action
the app can perform, behind one `verb entity` command grammar with
machine-readable `--json` output.

> **Unofficial.** Not affiliated with Verizon or Smith Micro. It automates *your
> own* family account over the same backend the app uses — the same idea as the
> community clients for Google Family Link, Microsoft Family Safety, and Nintendo
> parental controls. You are responsible for using it only on accounts you
> administer.

## Status

Early. The command surface and data model are generated from a protocol descriptor
(see `docs/FINDINGS.md`). A full dynamic capture confirmed the backend has **no
per-request device attestation**, that authenticated API calls use a bearer-style
`id_token`, and that login is a hybrid, multi-stage flow — so a durable scripted
client is viable.

Working today (no credentials needed):

```console
$ safe_cli entities
ENTITY             OPS  SUMMARY
account            2    The Verizon Family account (subscriber) and its lines.
web_filter         6    Website content filtering: category filters, allow/block lists, safe search.
screen_time        4    Daily screen-time limits and schedules (bedtime/offtime/downtime).
location           1    Real-time location of a profile's device.
...

$ safe_cli describe web_filter
web_filter — Website content filtering: category filters, allow/block lists, safe search.
  id: profileId
OP                    METHOD  PATH                                            CONFIRMED
block_site (action)   POST    /frisco/parental-control/v5/website             no (static)
...
```

Confirmed live end-to-end: `auth login` (the hybrid device-OTP + assisted My Verizon
web login + token exchange) authenticates against the production backend, and
authenticated calls succeed with the returned `id_token`. See **[Logging in](#logging-in)**.

Coming next: the generated `print/info/create/update/delete` + action commands
(`pause`, `block-site`, `locate`, …), the SigV4/Cognito client for the
parental-control endpoints, `analyze` (static APK triage), and `capture` (mitmproxy
addon).

## Logging in

`safe_cli auth login` is a one-time flow that fuses **two independent factors** into a
durable token set — control of a **line** on the account and the **account owner's** web
login. Full detail: `docs/PROCESS.md` §9.

1. **Device OTP (factor 1, signed).** A signed `.../v7/user/auth/otp` call texts a code to
   a line on the account; validating it (`.../otp/validate`) returns a short-lived
   `loginRecommendation` JWT — the *recom* token, valid ~30 min.
2. **Account web login (factor 2, browser).** The OAuth `authorize` URL redirects to the
   Akamai-protected *My Verizon* login (username + password + account 2FA). On success the
   browser is 302'd to the app's custom scheme `vsfapp://…/signin?code=…`; that
   authorization `code` is what we capture.
3. **Token exchange (the fusion).** `POST .../v7/user/auth/token` with the `code`, its PKCE
   `codeVerifier`, **and** the recom `token` mints the real `id_token` / `access_token`
   plus a 24 h **offline** `refresh_token`. Thereafter `safe_cli auth refresh` renews the
   `id_token` from the stored online refresh token — no browser or OTP (the token endpoint
   requires an `x-trace-transaction-id` header and a `friscoTokenType` that matches the
   refresh token's type, or it answers 400).

### The browser leg and Browserbase

Step 2 is the only part that can't run headless from a datacenter: Akamai Bot Manager
fingerprints the TLS/JA3 + sensor JS + IP reputation and blocks datacenter automation.
Two ways through:

- **Local browser (normal use).** Open the `authorize` URL in any real browser on a
  residential connection and sign in.
  - **macOS:** `safe_cli auth login` registers a tiny `vsfapp://` handler app with Launch
    Services (`safe_cli auth register-scheme` does it standalone), so the browser's
    `vsfapp://…?code=` redirect is delivered straight back to the waiting CLI — no paste.
    Test the handler alone with
    `open "vsfapp://com.verizon.familybase.parent/signin?code=TEST&state=x"`, then
    `cat "$HOME/Library/Application Support/safe_cli/vsfapp_redirect"`.
  - **Other platforms / `--paste`:** the OS can't open the custom scheme, so copy the
    `vsfapp://…?code=` URL from the address bar (or the "open external app?" prompt) and
    paste it back when `auth login --paste` asks.
- **Browserbase (headless / CI, what we drive here).** [Browserbase](https://browserbase.com)
  is a cloud browser on residential IPs with stealth on — enough to pass Akamai's page
  load. Drive it over CDP (Playwright `connect_over_cdp`): navigate the `authorize` URL,
  fill the credentials, submit the 2FA, and intercept the `vsfapp://` navigation via
  `Network.requestWillBeSent`. Breadcrumbs that cost real time:
  - Create the session with `proxies:true, keepAlive:true, timeout:1800` — the **default
    5-minute timeout** will cut you off mid-2FA.
  - Only the `authorize → login → 2FA → vsfapp://code` hop goes through the browser; the
    signed device-OTP legs and the token exchange run **directly** from the CLI against the
    frisco API (Akamai only guards the My-Verizon web login).
  - Re-navigating `authorize` in an already-logged-in session can land on a **stale 2FA
    page that never sends a code** — start a fresh session (no cookies) to force the full
    username → password → 2FA and a real code.
  - Auth `code`s expire in ~60 s; exchange immediately. The recom token lasts ~30 min, so
    do the device-OTP leg close to the browser leg.

  Full recipe and footguns: `docs/PROCESS.md` §9a.

The durable **offline** `refresh_token` (24 h) is stored `0600`; day-to-day calls use the
`id_token`, renewed by `safe_cli auth refresh` with no browser or OTP.

## Build

```bash
make build      # -> bin/safe_cli   (Go 1.22+, no CGO)
make test
make lint
```

## Design

- **Descriptor-driven.** `internal/descriptor/verizon_family.json` is the single
  source of truth; the CLI surface and data model derive from it. Endpoint paths
  discovered by static analysis carry a `confirmed` flag a live capture flips.
- **Modeled on the OpenClaw/steipete Go CLIs** (`gog`, `wacli`): `kong` grammar,
  `--json` stdout-as-API, self-contained binary, safety profiles (planned).

## Contributing

All changes land via PR through a review gate (`.claude/skills/pr-review-gate`)
that requires every review thread resolved **and** disposed, with Codex reviewing
each PR. See `CLAUDE.md`.

## Docs

- `docs/FINDINGS.md` — the deciding-question verdict and evidence.
- `docs/PROCESS.md` — the full reverse-engineering + dynamic-capture runbook.
- `docs/discovered-endpoints.txt` — the 183-endpoint catalog.

## License

MIT — see `LICENSE`.
