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
`docs/FINDINGS.md`). A dynamic capture (via **eCapture** — eBPF, reading conscrypt
plaintext after TLS decrypt) confirmed the backend has **no per-request device
attestation** and that authenticated API calls send the raw `id_token` in the
`Authorization` header (**no `Bearer` prefix**; not SigV4/Cognito — a debunked red
herring). `auth login` authenticates live, `auth refresh` renews without a browser,
and the descriptor's 59 entities / 459 operations are all invokable through `call`.
Reads and reversible mutations have been verified against production, and mutation
request bodies checked byte-for-byte against the app's own captured traffic.

### Quick start (built for agents)

The surface is designed to be assembled from introspection — no memorization:

```console
$ safe_cli entities                     # the whole data model
$ safe_cli describe content_filter      # one entity's ops: names, method, flags, what each does
$ safe_cli members                      # the family, with the ids you target
NAME    ROLE       SERVICE-ID  PROFILE-ID  DEVICE-ID  PAIRING
Parent  GUARDIAN   1000001     2000001     3000001
Kid     DEPENDENT  1000002     2000002     3000002    UNPAIRED
```

(Example rows — synthetic names and ids.)

`members` is the intended first call once logged in: it tells you which
`--service-id` to pass (a **DEPENDENT** is a managed child — pass the child's service
id, e.g. `1000002` above, not your own).

`describe` names, per op, exactly what to supply — `svc` (needs `--service-id`),
`body` (needs `--data`), `query=<names>` / `header=<names>` / `path=<names>` (the
exact `--query`/`--header`/`--path name=value` args), and `⚠ …confirm` for a
catastrophic op that refuses without `--confirm`. Then `call` runs it:

```console
$ safe_cli call schedules getSchedules --service-id 1000002 --json
$ safe_cli call app_block blockApp --service-id 1000002 \
    --data '{"subcategory":{"name":"Social","id":101,"enabled":true,"categoryId":5,"categoryShortName":"SOC"}}'
```

With `--data` omitted, an op that needs a body prints a worked example; every error
says exactly which flag to add. `--dry-run` prints the request without sending it —
as an HTTP/1.1 *rendering* (that's just how Go's `httputil.DumpRequestOut` serializes);
the wire is actually HTTP/2, same as the app. Everything speaks `--json` for
stdout-as-API.

## Logging in

`safe_cli auth login` is a one-time flow that fuses **two independent factors** into a
durable token set — control of a **line** on the account and the **account owner's** web
login. Full detail: `docs/PROCESS.md` §8–§9.

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

### The browser leg (factor 2)

Factors 1 and 3 (device OTP, token exchange) run **directly** from the CLI against the frisco
API — no browser. Only the middle leg — the *My Verizon* account login + 2FA — sits behind
Akamai Bot Manager, which fingerprints TLS/JA3 + sensor JS + IP reputation and blocks plain
datacenter automation. You complete that leg in a **real browser** and hand the resulting
`vsfapp://…?code=` redirect back to the CLI.

**On the device-OTP step:** the backend returns `2xx` even on failure — only `state=="OTP_SENT"`
actually texts a code (`OTP_ATTEMPTS_EXCEEDED`, `OTP_DISPATCH_FAILED`, … are `2xx` too), and the
CLI surfaces the real state instead of hanging. Each `auth login` sends a **new** OTP that
supersedes the last, so on a retry reuse the code already delivered: `auth login --otp <code>`
skips the request and validates that code directly.

**The CLI drives this leg over stdin:** `--no-browser` suppresses the auto-open and `--paste`
takes the redirect by hand. The OTP and the pasted redirect arrive at different times, so drive
it step-wise — e.g. feed stdin through a FIFO so the two inputs can span separate steps.

- **Android emulator as the browser (what we use).** An emulator's in-process Chromium WebView
  presents a real browser fingerprint and passes Akamai **even on a datacenter host** — so the
  residential-IP requirement applies to plain datacenter automation, not this. Load the
  `authorize` URL into it and drive the login over `adb`:
  ```bash
  adb shell am start -a android.intent.action.VIEW -d '<authorize-url>'   # org.chromium.webview_shell
  # …enter user id + password + the account 2FA in the WebView…
  # the vsfapp://…?code= redirect lands in Android's app-chooser (ResolverActivity):
  adb shell dumpsys activity activities | grep -oE 'vsfapp://[^ }]*'      # grab the code
  adb shell input keyevent KEYCODE_BACK                                   # BACK — so the app does
                                                                          # NOT consume the one-time code
  ```
  Paste that full `vsfapp://…?code=…` URL into `auth login --no-browser --paste`.
- **A normal local browser (end-user use).** Open the `authorize` URL in any real browser on a
  residential connection and sign in.
  - **macOS:** `safe_cli auth login` registers a tiny `vsfapp://` handler with Launch Services
    (`safe_cli auth register-scheme` does it standalone), so the browser's redirect is delivered
    straight back to the waiting CLI — no paste. Test it with
    `open "vsfapp://com.verizon.familybase.parent/signin?code=TEST&state=x"`, then
    `cat "$HOME/Library/Application Support/safe_cli/vsfapp_redirect"`.
  - **Other platforms:** copy the `vsfapp://…?code=` URL from the address bar (or the "open
    external app?" prompt) and paste it back when `auth login --paste` asks.
- **Browserbase (secondary / CI).** [Browserbase](https://browserbase.com) is a cloud browser on
  residential IPs with stealth — an alternative to the emulator for headless CI. Drive it over CDP
  (Playwright `connect_over_cdp`): navigate `authorize`, fill credentials, submit 2FA, intercept
  the `vsfapp://` navigation via `Network.requestWillBeSent`. Footguns: create the session with
  `proxies:true, keepAlive:true, timeout:1800` (the default 5-min timeout cuts you off mid-2FA);
  re-navigating `authorize` in an already-logged-in session can land on a **stale 2FA page that
  never sends a code** — start a fresh cookieless session; auth `code`s expire in ~60 s (exchange
  immediately), and the recom token lasts ~30 min so do the device-OTP leg close to the browser leg.

Full recipe and footguns: `docs/PROCESS.md` §9 (the recipe) and §13 (footguns).

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
- `docs/PROCESS.md` — the full reverse-engineering + dynamic-capture (eCapture) runbook,
  including the login recipe and the parity-verification methodology.
- `docs/api-catalog.md` — human-readable per-op catalog (method, path, identity headers);
  `docs/vsf-endpoints.json` is the machine-readable form.
- `docs/discovered-endpoints.txt` — the early raw path harvest (183 paths).

## License

MIT — see `LICENSE`.
