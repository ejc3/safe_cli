---
name: pr-review-gate
description: "ALWAYS USE THIS SKILL when: a PR has review findings (Codex, a bot, or a human), before resolving any review thread, before calling a PR green, and before merging. Enforces that a defect claim is closed by a test that fails without the fix (RED-VERIFIED), not by the fix alone. Invoke with /pr-review-gate"
allowed-tools: Bash, Read, Grep, Glob
user-invocable: true
---

# Adjudicating PR review findings

## Quick Reference

| Task | Command |
|------|---------|
| Check threads (this gate) | `.claude/skills/pr-review-gate/check-review-threads.sh <N>` |
| Read inline findings (Codex) | `gh api repos/$REPO/pulls/<N>/comments --jq '.[] \| "---\nfile: \(.path):\(.line // .original_line)\n\(.body)"'` |
| Read PR-level review bodies | `gh api repos/$REPO/pulls/<N>/reviews --jq '.[] \| "---\n\(.user.login) \(.state)\n\(.body)"'` |
| Trigger a Codex review | `gh pr comment <N> --body "@codex review"` |
| Prove checks exist, not just pass | see "Prove the checks EXIST" below |
| Merge | `gh pr merge <N> --squash` (branch auto-deletes) |

**Setup** — once per session:
```bash
REPO=$(gh repo view --json nameWithOwner --jq '.nameWithOwner')
```

---

Merging here is gated on review-thread resolution. That gate can tell you a thread is
*resolved*; it cannot tell you the finding was *answered*. This skill is how you answer
one.

## The rule: a defect claim is closed by a RED test, not by a fix

**Whenever anyone — Codex, a bot, a teammate, you — says "this is broken", the thing that
closes it is a test that FAILS WITHOUT THE FIX.** Not the fix. Not "verified manually".
Not a green suite after the change, which proves only that the suite never covered it.

Applies to every source of a defect claim, not just review comments: a CI failure, a bug
report, a hunch, a comment you wrote yourself.

**The procedure, in order:**

1. Write the test. Run it against the **unfixed** tree. **Watch it fail.**
2. Apply the fix. Watch it pass.
3. Revert the fix once more and confirm it goes red again — if you skipped step 1, this
   is your last chance to learn the test was vacuous.
4. Only then resolve the thread, replying `RED-VERIFIED: <test name or path>`.

A test written after the fix, never observed failing, is indistinguishable from a test
that cannot fail. That is not hypothetical: this rule comes from `ejc3/fcvm`, which
repeatedly found checks that could never fire — a leak check whose pattern never
matched, a `grep '^ *FAIL'` blind to the runner's actual failure format, a branch logged
0 times across 137 runs. Every one was green for its whole life.

The trap is live in this stack too. A Jest suite passes happily when `moduleNameMapper`
points at a file that does not exist yet; `pytest` reports success on a file it never
collected. Watching the test fail first is what separates "this is covered" from "this
looks covered".

**Every resolved thread needs one of three dispositions as a reply.** The gate does not
try to guess which findings are defects — guessing failed in both directions, letting
ordinary phrasing like "this drops the final game" close on an assertion, while making a
*wrong* defect claim impossible to ever close. Answer each one instead:

| Disposition | Use for |
|---|---|
| `RED-VERIFIED: <test>` | a defect claim, closed by a test you watched fail without the fix |
| `NOT-A-DEFECT: <reason>` | naming, docs, style — say what you changed |
| `DISAGREE: <reason>` | a defect claim you are rejecting, with the reasoning |

Disagreement is a legitimate resolution; silence is not. The text after the colon must be
non-empty — a bare `RED-VERIFIED:`, or a comment merely *asking* someone for proof, is not
a disposition.

## Check the threads

```bash
.claude/skills/pr-review-gate/check-review-threads.sh <pr-number>
```

The gate has its own tests — `test-check-review-threads.sh`, one case per past failure,
each written against the unfixed script and observed failing first:

```bash
.claude/skills/pr-review-gate/test-check-review-threads.sh \
  .claude/skills/pr-review-gate/check-review-threads.sh
```

Exit codes: `0` clear, `1` blocked (unresolved threads, or a resolved defect claim with
no `RED-VERIFIED` reply), `2` the gate could not evaluate — treat 2 as blocked, never as
a pass.

It flags three things:

- **UNRESOLVED** — a finding nobody has answered. Not a finding that is wrong.
- **UNDISPOSED** — a resolved thread with no disposition reply. Resolving without
  replying closes a finding without answering it.
- **UNACKED** — a PR-level **review body** nobody acknowledged. A defect claim can arrive
  in the review body rather than an inline comment; that is not a thread, has no
  "resolved" state, and never appears in `reviewThreads`. Reply on the PR with
  `REVIEW-ACK: <what you did>` once you have read and answered it.

It also blocks when it cannot trust its own input: a PR number that does not resolve, a
thread whose `isResolved` is not a boolean, or a missing `jq`/`gh`. Those exit `2`.

## Prove the checks EXIST before trusting green

"No failures" is not "it passed". A check set can be green because nothing ran.

```bash
SHA=$(gh api repos/$REPO/pulls/<N> --jq .head.sha)
gh api repos/$REPO/commits/$SHA/check-runs \
  --jq '.check_runs[] | select(.conclusion != "skipped") | "\(.name)\t\(.conclusion)"' | sort -u
```

Expect both `go (build, vet, test, lint)` and `golangci-lint`. **A job missing from
that list is a failure to verify, not a pass.**

In `ejc3/fcvm` a PR merged on a "no failures" reading whose head sha had **zero** runs of
the real jobs: a `pull_request: branches: [main]` filter silently skipped the workflow
for every PR whose base was not `main`, and the only check that ran was an unrelated one.
It carried three lint violations.

`ci.yml` here uses a bare `pull_request:` trigger with **no `branches:` filter** so
stacked PRs are covered. Do not add one.

## A gate must fail closed

A check that cannot run must **block**, never pass. Passing is a claim, and a tool that
could not evaluate anything has no basis for making it. If a dependency is missing, the
input is malformed, or the API is unreachable, exit non-zero — never default to clear. A
gate that waves everything through precisely because it broke is strictly worse than no
gate, because it looks like one.

This applies to your own verification too: if you could not run the check, say so. Do not
report a verdict you did not earn.

## Before you merge

1. Every thread resolved **and disposed** (`RED-VERIFIED:` / `NOT-A-DEFECT:` / `DISAGREE:`).
2. Every PR-level review body acknowledged (`REVIEW-ACK:`).
3. Both required checks present **and** passing.
4. Branch up to date with `main` (the ruleset enforces this).
5. Squash-merge; the branch deletes itself.

Run `check-review-threads.sh <N>` to confirm 1 and 2 rather than eyeballing them.

The oversized-thread re-fetch cannot be covered by `--from-file` fixtures — they contain
every comment inline, so they pass with or without it. `probe-pagination.sh` verifies that
path against a real PR with `COMMENTS_PAGE_SIZE=1`, comparing three merges: no re-fetch and
`unique_by` (which sorts, hiding a disposition that sorts before the finding) must both MISS
the disposition; the shipping order-preserving merge must see it.
