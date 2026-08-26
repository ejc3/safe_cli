# CLAUDE.md — safe_cli

Project context and rules for AI coding agents. `AGENTS.md` is a symlink to this
file so non-Claude tooling (Codex, etc.) reads the same instructions.

## What this is

`safe_cli` is a comprehensive, **gog/GAM-style** command-line tool for **Verizon
Family** (the white-label deployment of **Smith Micro SafePath**). One monolithic
`verb entity` grammar exposes the entire SafePath data model and every action the
app can perform. It is **unofficial** and intended for a person administering
**their own family account** (interoperability / automation), comparable to the
existing community clients for Google Family Link, Microsoft Family Safety, and
Nintendo parental controls.

Written in **Go**, modeled on the OpenClaw / steipete CLI conventions (`gog`,
`wacli`, …): `kong` for the command grammar, `--json` stdout-as-API,
descriptor-driven surface, self-contained static binary. (Full gog/wacli-style
safety profiles are *planned*; the shipped guard today is that destructive ops
refuse to run without `--confirm`.)

## Status (2026-08-26)

- **Deciding question answered: VIABLE.** Static analysis of the signed APK
  (v8.101.30) found the backend and a **plain OAuth2/OIDC bearer** auth model.
  The Play Integrity references belong to the optional Nok Nok FIDO login path,
  **not** a per-request gate; no Firebase App Check; no attestation header on the
  API. TLS pinning is present (affects capture only). See `docs/FINDINGS.md`.
- **Backend:** `https://api.prd.vsf.aws.vz-connect.com` — parental-control API at
  `/frisco/parental-control/…` and `/vsf/…`. Full catalog (183 paths):
  `docs/discovered-endpoints.txt`.
- **Built (working live against prod):** the full descriptor-driven surface —
  `entities`/`describe`/`members` introspection, the full `auth` command
  (`login`/`refresh`/`import`/`status`/`logout`/`extract-key`, plus `register-scheme`
  on macOS; durable, auto-refresh-on-401), `raw` (invoke any backend path with the
  stored id_token), and `call` (the generic invoker for all 459 ops, with `--dry-run`).
  Reads and reversible mutations verified against production; mutation bodies checked
  byte-for-byte against captured app traffic.
- **Deliberately dropped (not planned):** the once-envisioned `analyze` (Go port of
  the static APK analyzer) and `capture` (mitmproxy addon) subcommands. Both were
  descriptor-*maintenance* tooling, not needed to operate an account, and overtaken
  by how the RE actually went: the static analysis is complete and encoded in the
  descriptor (its one reusable piece, dex parsing for the signing key, is already in
  `internal/apkkey`); and mitmproxy is the wrong capture tool here (TLS pinning +
  native anti-frida), so we used **eCapture** (eBPF, conscrypt uprobes) instead and
  confirmed ops + filled bodies directly. If a `capture`-style helper is ever wanted,
  it would be a small "parse an eCapture dump and diff it against the descriptor"
  tool — a different thing from the planned mitmproxy addon.
- **Credentials** (the user's Verizon login) are needed only at the auth/live
  step — never committed.

## Repo governance (same rigor as dolphin-labs)

- Canonical remote: `https://github.com/ejc3/safe_cli` (public).
- **All changes land via pull request; do not push directly to `main`.**
- **Every PR conversation must be resolved _and_ disposed before merging** with
  one of `RED-VERIFIED:` / `NOT-A-DEFECT:` / `DISAGREE:` (see the
  `pr-review-gate` skill). PR-level review bodies need a `REVIEW-ACK:`.
- **Codex reviews every PR**: `gh pr comment <N> --body "@codex review"`.
- No human approver is required — the author may self-merge once conversations
  are resolved+disposed and checks pass.
- **A defect claim is closed by a test that FAILS WITHOUT THE FIX (RED-VERIFIED),
  not by the fix alone.** Never skip or comment out a failing test — fix the root
  cause.
- **Prove the checks EXIST, not just green** — a job missing from a head sha's
  check-runs is a failure to verify, not a pass. `ci.yml` uses a bare
  `pull_request:` trigger (no `branches:` filter) so stacked PRs are covered;
  do not add one.

Run the gate before calling a PR green or merging:

```bash
.claude/skills/pr-review-gate/check-review-threads.sh <pr-number>
```

## Layout

- `cmd/safe_cli/` — CLI entrypoint (kong grammar) + every command.
- `internal/descriptor/` — the embedded protocol descriptor + loader; the single
  source of truth for the command surface and data model.
- `internal/client/` — the HTTP/2 SafePath client: id_token API calls, the signed
  device-auth (OTP) legs, and the unsigned token refresh/exchange.
- `internal/oauth/` — PKCE + the OAuth authorize-URL builder.
- `internal/tokenstore/` — the `0600` on-disk token store (online/offline set).
- `internal/signing/` — the HMAC `x-signature` request signer (bring-your-own-key).
- `internal/apkkey/` — dex parser that extracts the signing key from your own APK.
- `internal/deviceid/` — the persisted per-install `app_uuid`.
- `internal/outfmt/` — table + JSON output.
- `docs/` — `PROCESS.md` (the end-to-end RE + dynamic-capture methodology runbook),
  `FINDINGS.md` (deciding-question writeup), `api-catalog.md` + `vsf-endpoints.json`
  (the harvested per-op catalog), `discovered-endpoints.txt`.
- `.claude/skills/pr-review-gate/` — the review-thread gate (ported from
  dolphin-labs, retargeted to this repo; its own RED-verified self-tests pass).

## Commands

```bash
make build      # -> bin/safe_cli
make test       # go test -race ./...
make lint       # go vet + go mod tidy check + gofmt check + golangci-lint (17 linters)
make vuln       # govulncheck ./... (also gated in CI)
make fmt        # gofmt -w
./bin/safe_cli entities
./bin/safe_cli describe content_filter
```

Before pushing: `make lint` and `make test` must pass.

## Secrets

Never commit: the APK or anything extracted from it (`*.apk/*.xapk/*.dex`), `.env`,
OAuth tokens, the user's Verizon credentials, or any per-user / per-session value
(device `appUUID`, transaction ids, timestamps, and per-user service/profile/device
ids or member names) — in code **or tests, docs, and commit messages**. All are
git-ignored; tests use synthetic inputs with independently-computed expected values,
and a README test guards its examples against real ids. `.gitignore` uses
**root-anchored** patterns (`/bin/`, `/dist/`) — never bare dir-name patterns, which
would also ignore any same-named path deeper in the tree.

**The app's HMAC request-signing key is NOT committed.** It is the vendor's shared
build constant; publishing interoperability *code* is different from redistributing a
live shared credential, and there is no clean authority that committing it is lawful.
So `internal/signing` ships the algorithm only, keyed by a secret the operator
supplies at runtime from their **own licensed copy of the app** (extraction is a local
step the operator runs — see `docs/PROCESS.md` §7). The CLI never writes the key to
disk or logs it; the one exception is `auth extract-key`, whose explicit purpose is to
print the key you asked it to read from your APK.

**The one embedded constant is the OAuth `client_id`** — a public OAuth *client
identifier*, not a secret: it grants nothing on its own, and a real login still needs
the owner's Verizon credentials + SMS/2FA.
