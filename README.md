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

Early. The command surface and data model are generated from a protocol
descriptor built by static analysis of the app (see `docs/FINDINGS.md`), which
established that the SafePath backend uses **plain OAuth2 bearer auth with no
per-request device attestation** — so a durable scripted client is viable.

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

Coming next: `auth login` (OAuth2/PKCE), the generated
`print/info/create/update/delete` + action commands (`pause`, `block-site`,
`locate`, …), `analyze` (static APK triage), and `capture` (mitmproxy addon).

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

## License

MIT — see `LICENSE`.
