<!-- prose:plain -->
## What

<!-- The change, in one or two sentences. What behaviour is different after this PR? -->

## Why

<!-- The reason this change exists. Link the issue or decision record it
     implements. State the invariant, not the fix narration. -->

## How verified

<!-- The gates you ran and what they proved. At minimum: `make check` green, and
     `make test-integration` if this touches tenant data, workspace isolation or the write shape.
     Name the manual flow you drove if the change has a runtime surface. -->

- [ ] `make check` is green
- [ ] `make test-integration` is green (or: this change does not touch tenant data, workspace isolation or the write shape)
- [ ] `make craft-static` reports no new `BLOCKER` findings
- [ ] This diff and the PR body hold no secrets, no customer data and no local machine paths. They hold nothing copied from a private document or pointing at one. A decision is named only by its number (this repository is public)

## AI involvement

<!-- Which parts were AI-assisted, and how. This repo is built by agents under
     human accountability; say what was generated and what was hand-written. -->

## Who is accountable

By opening this PR, you confirm that you are **accountable** for this change. You can
**explain every line** in it, whether a human or an AI wrote it. See
[CONTRIBUTING.md](/CONTRIBUTING.md) and the
[Code of Conduct](/CODE_OF_CONDUCT.md).
