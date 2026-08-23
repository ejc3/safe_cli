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
descriptor-driven surface, safety profiles, self-contained static binary.

## Status (2026-08-23)

- **Deciding question answered: VIABLE.** Static analysis of the signed APK
  (v8.101.30) found the backend and a **plain OAuth2/OIDC bearer** auth model.
  The Play Integrity references belong to the optional Nok Nok FIDO login path,
  **not** a per-request gate; no Firebase App Check; no attestation header on the
  API. TLS pinning is present (affects capture only). See `docs/FINDINGS.md`.
- **Backend:** `https://api.prd.vsf.aws.vz-connect.com` — parental-control API at
  `/frisco/parental-control/…` and `/vsf/…`. Full catalog (183 paths):
  `docs/discovered-endpoints.txt`.
- **Built:** descriptor (`internal/descriptor`) + introspection commands
  (`version`, `entities`, `describe`). **Next:** OAuth flow + live client + the
  generated `print/info/create/update/delete` + action commands, then `analyze`
  (Go port of the static analyzer) and `capture` (mitmproxy addon).
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

- `cmd/safe_cli/` — CLI entrypoint (kong grammar).
- `internal/descriptor/` — the embedded protocol descriptor + loader; the single
  source of truth for the command surface and data model.
- `internal/outfmt/` — table + JSON output.
- `docs/` — `FINDINGS.md` (static-analysis writeup), `discovered-endpoints.txt`.
- `.claude/skills/pr-review-gate/` — the review-thread gate (ported from
  dolphin-labs, retargeted to this repo; its own RED-verified self-tests pass).

## Commands

```bash
make build      # -> bin/safe_cli
make test       # go test -race ./...
make lint       # gofmt check + go vet + golangci-lint
make fmt        # gofmt -w
./bin/safe_cli entities
./bin/safe_cli describe web_filter
```

Before pushing: `make lint` and `make test` must pass.

## Secrets

Never commit: the APK or anything extracted from it (`*.apk/*.xapk/*.dex`),
`.env`, OAuth tokens, or the user's Verizon credentials. All are git-ignored. The
only extracted value in the repo is the app's public OAuth `client_id`.
