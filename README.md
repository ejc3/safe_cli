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

Working end-to-end against the production backend. The command surface and data
model are generated from a protocol descriptor (`internal/descriptor`,
`docs/FINDINGS.md`). A full dynamic capture confirmed the backend has **no
per-request device attestation** and that authenticated API calls use a
bearer-style `id_token` (not SigV4/Cognito — a debunked red herring). `auth login`
authenticates live, `auth refresh` renews without a browser, and the descriptor's
59 entities / 459 operations are all invokable through `call`. Reads and reversible
mutations have been verified against production, and mutation request bodies checked
byte-for-byte against the app's own captured traffic.

### Quick start (built for agents)

The surface is designed to be assembled from introspection — no memorization:

```console
$ safe_cli entities                     # the whole data model
$ safe_cli describe web_filter          # one entity's ops: names, method, flags, what each does
$ safe_cli members                      # the family, with the ids you target
NAME           ROLE       SERVICE-ID  PROFILE-ID  DEVICE-ID  PAIRING
EJ             GUARDIAN   9236178     10321597    11682544
Colin's Phone  DEPENDENT  4833687     5430944     6430944    UNPAIRED
```

`members` is the intended first call once logged in: it tells you which
`--service-id` to pass. A **DEPENDENT** is a managed child; parental-control ops act
on the child, so you pass the *child's* service id, not your own.

`describe` names, per op, exactly what to supply — `svc` (needs `--service-id`),
`body` (needs `--data`), `query=<names>` / `header=<names>` / `path=<names>` (the
exact `--query`/`--header`/`--path name=value` args), and `⚠ …confirm` for a
catastrophic op that refuses without `--confirm`. Then `call` runs it:

```console
$ safe_cli call schedules getSchedules --service-id 4833687 --json
$ safe_cli call app_block blockApp --service-id 4833687 \
    --data '{"subcategory":{"name":"Diply","id":10029,"enabled":true,"categoryId":1005,"categoryShortName":"SOC"}}'
```

With `--data` omitted, an op that needs a body prints a worked example; every error
says exactly which flag to add. `--dry-run` prints the exact request without sending
it. Everything speaks `--json` for stdout-as-API.

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
   `id_token` from the stored refresh token — online preferred, the durable offline token
   as fallback — with no browser or OTP (the token endpoint requires an
   `x-trace-transaction-id` header and a `friscoTokenType` that matches the refresh token's
   type, or it answers 400).

### The browser leg and Browserbase

Step 2 is the only part that can't run headless from a datacenter: Akamai Bot Manager
fingerprints the TLS/JA3 + sensor JS + IP reputation and blocks datacenter automation.
Two ways through:

If the OTP request fails, the CLI now says why (the backend returns 2xx with a
`state` even on failure — `OTP_ATTEMPTS_EXCEEDED`, `OTP_DISPATCH_FAILED`, … — and
only `OTP_SENT` actually texts a code). Because each `auth login` sends a *new* OTP
that supersedes the last, a retry should reuse the code already delivered:
`auth login --otp <code>` skips the request and validates that code directly.

- **Local browser (normal use).** Open the `authorize` URL in any real browser on a
  residential connection and sign in. "Any real browser" includes an **Android
  emulator's** browser — load the `authorize` URL there, complete the login + 2FA,
  and the `vsfapp://…?code=` redirect surfaces in the app-chooser intent
  (`adb shell dumpsys activity activities | grep -o 'vsfapp://[^ }]*'`) to paste back.
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
