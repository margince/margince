<!-- prose:plain max-words=1000 -->
# Contributing to Margince

Margince is source-available under the Business Source License (BUSL-1.1). AI agents write most of
its code, and a human is accountable for each change. Your changes are welcome, and they must meet the
same rules as the code our own agents write.

The [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) covers how we work together. It adds one rule:
to send more code than you can explain wastes the time of the reviewers.

## You answer for your change

**You are accountable for every line you send.** You must explain every line. We expect you
to use AI help. We refuse a change you cannot explain, and more code than the task needs. If you cannot say
why a line is there, what it does and why it is right, it is not ready. "The model wrote it" does not
answer a review question.

This rule never changes. The AI note below keeps a human accountable, and it does not stop you from
using AI.

## Say how you used AI

In the pull request text, say how you used AI:

- **Assisted**: you wrote it, and AI helped you write, review or change the shape of code. This is the normal case.
- **Generated**: AI wrote much of it, and you read it all and own it.

Our own agents do not add this note, because all their code is AI code. You add it so the reviewer knows
what you answer for. The same checks run in both cases.

## Start the stack

You need Go 1.27 or later, Docker, `golangci-lint`, and Node with pnpm for the web app. The
`packageManager` field in `package.json` sets the pnpm version, and both Corepack and pnpm switch to it.

```
make install    # FE deps, gate tools, and the git hooks — run once
make db-up      # PG16 + Redis 7 containers, and the app role
make migrate    # core + custom migrations
make dev        # the whole stack: the app on :8080, the api behind it, worker
```

Two things to know about `make dev`:

- It starts the stack of this worktree only, so two copies can run at once. Only `make dev-sweep` stops
  every stack on the machine.
- Vite shows web app changes as you type, but the API is a built program. After a backend change, run
  `make dev`: it stops the old stack of this worktree and starts a new one. An old API looks the
  same as a bug.

[docs/reference/make-targets.md](docs/reference/make-targets.md) lists every command.

## Branch, commit, merge

- Make a branch from `main`: `git switch -c <type>/<slug> origin/main`. You cannot push to `main`, and
  there is no other way to merge.
- Write each commit subject in the Conventional Commits form, with the module as its scope:
  `fix(deals): a closed deal reports the stage it closed in`. Say how the product works after the
  change, not the task you worked on.
- We merge with squash, and only when every check is green.

### Contributing from a fork

A pull request from a fork runs the same `ci` checks as our own work, and `ci` is the one check a merge
needs. A maintainer approves the run before it starts, so expect a short wait. A fork run gets no SonarCloud
token, so SonarCloud does not report, and a merge does not need it.

## The checks

Code, docs and settings all go through the same checks.

- `make check` is the merge check. It builds, lints and tests the code. It also checks the generated
  code, changes to the API contract, the image pins and file length, and the web app (`check-fe`).
- `make test-integration` is a separate run against a real Postgres, and needs `make db-up`. It tests
  that a workspace sees only its own data, GDPR erase requests, and an audit log no one can change. With
  no database it fails and does not skip, because a skipped security check looks the same as a passed one.
- The craftsmanship gate (`craft static --strict`) runs on each push once you run `make hooks`. It blocks
  each `BLOCKER` and `MAJOR` finding in the Go code you changed and in the comments you add to Go or
  TypeScript. A `MINOR` finding does not block.
- Every check in CI must be green before a merge: the same checks, plus review bots and SonarCloud. Fix
  what they find, and do not dismiss it.

The craftsmanship gate finds a dropped error, a sleep in a test, and a test that checks nothing. It
also finds an `any` type or two `bool` values in a signature, and a function over 80 code lines (160 in
`*_test.go`). A line with only a comment does not count against that limit, but a comment after code
does not free its line.

`make craft-static` is green on `main`, and CI runs the same check. If a finding is wrong, mark it in the source with a reason: `//craft:ignore <check> <reason>`.
That is the only way to stand a finding down, and the next developer to touch the line reads the reason.
`scripts/craft-pin.sh` gets the tool and checks its checksum, so your laptop and CI agree.

Write it right the first time. Match the file you change. A comment says why, not what. Never
drop an error. A test must prove how the code works, or it is noise.

## Where things go

- What you decide in your work goes in the commit message and the pull request text. The git history
  is the record.
- Open work is in GitHub issues, and there is no status file. Start there, and read
  [AGENTS.md](AGENTS.md) for the rules every change must follow.
- Each GitHub issue starts from a form: a bug, a deferred
  follow-up, a missing feature or proposal, or a problem in the docs.
- A deferred follow-up is a problem you leave for later: an issue, not a comment in the code.
- A security problem goes in private, as [SECURITY.md](SECURITY.md) explains. Never open a public issue
  for it.
- This repository is public. Never put a secret, customer or personal data, a local path, or a private
  document or its path in an issue, pull request or commit. Write the rule out instead of pointing to
  something a reader cannot open.

`backend/api/crm.yaml` is the API contract, and the code follows it. The running software, its
contract, its tests and its docs come first. If an older document disagrees with the code, name the
conflict and keep going.
[docs/principles/the-record-is-the-code.md](docs/principles/the-record-is-the-code.md) explains why.

## Before you open a pull request

- Keep the pull request small, and make it tell the story: what changed, why, and how you checked it.
- Check your change against the Craftsmanship section of [AGENTS.md](AGENTS.md).
- `make check` is green on your machine.
