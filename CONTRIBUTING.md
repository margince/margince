# Contributing to Margince

Margince is source-available (BUSL-1.1) and AI-native: most of this code
is authored by agents under human accountability. Contributions are
welcome — held to the same craftsmanship bar as our own AI-authored code.

Participation is covered by our [Code of Conduct](CODE_OF_CONDUCT.md),
which includes one clause specific to a repository built this way:
submitting volume you cannot explain is treated as disrespect for
reviewers' time, not as enthusiasm.

## Licensing and the Contributor License Agreement

Margince is source-available, not open source. The code is public and free to read, run and modify. Production use is free up to ten seats, and from the eleventh seat a commercial subscription applies. Nobody may host Margince as a service for others without a partner agreement. The full terms are in [LICENSE](LICENSE).

**Every release converts to the Apache License 2.0 two years after it ships.** Each version carries its own Change Date, stamped in the LICENSE file at release. On that date, that version becomes fully open source, permanently, for everyone. We describe Margince as "source-available, becomes Apache 2.0 in two years", and we do not describe it as open source before that point.

### Purpose of the agreement

Before we can merge your pull request, we need you to accept our [Contributor License Agreement](CLA.md). A bot will prompt you on your first pull request; accepting takes one click and covers everything you contribute afterwards.

We ask for this because of how Margince is licensed. We sell commercial subscriptions, we license hosting partners, and we convert each release to Apache 2.0 on schedule. Every one of those requires us to hold licensing rights over the whole codebase, including the parts written by outside contributors. A Developer Certificate of Origin would confirm that you wrote your code, but it would not give us those rights, and a single contribution without them would leave part of the product that we cannot sell, cannot relicense and cannot convert.

### Effect of the agreement

The CLA is a licence, not a transfer, so you keep the copyright in your contribution and remain free to use, publish and relicense it anywhere else on any terms you choose.

In return, we may license your contribution to others on any terms, including commercial ones. That grant is what makes the paid tiers and the scheduled Apache conversion possible, and it also means we are not contractually limited to the licence we use today. We intend to keep the two-year conversion for every release, and we state it here as published policy so that you can see what it is: a policy, visible in the Change Date of every release, rather than an individual promise to each contributor.

Your contribution comes as is. You give no warranty that it works, and you carry no liability for it.

### Before you open a pull request

Your contribution must be your own work. If it contains code written by someone else, or code under a different licence, say so clearly in the pull request.

Do not include dependencies licensed under the GPL, the AGPL, or any other licence with source-disclosure obligations. Margince follows a permissive-only dependency rule, and our build checks enforce it. A copyleft dependency anywhere in the tree, including one pulled in indirectly by another package, would prevent us from licensing the product as described above.

If you are contributing as part of your job, check that your employer is comfortable with it. Your employer may accept the CLA on behalf of its staff under Clause 7.

We ask every contributor to accept the CLA, including for small fixes and typo corrections. This reflects nothing about the size of the contribution. A record that carries exceptions does not establish a complete chain of rights, which is the only use the record has.

## Human accountability

**You are accountable for every line you submit, and must be able to
explain every line.** AI assistance is welcome and expected;
unexplainable, slop-flooded contributions are not. If you cannot explain
why a line is there, what it does, and why it is correct, it is not
ready. "The model wrote it" is not an answer to a review question.

This is the project's one non-negotiable. It is why we ask for the
disclosures below — not to discourage AI use, but to keep a human
answerable for the result.

## AI disclosure

Disclose AI involvement proportionately in the PR description:

- **Assisted** — you wrote/directed it with AI help (autocomplete,
  review, refactor). The default.
- **Generated** — AI produced substantial portions you then reviewed
  and own.

There is a deliberate **internal/external asymmetry**: Margince's own
build agents author by design and do not disclose per-PR (it is the
stated practice, A39); external contributors disclose so a human
reviewer knows what they are accountable for. Either way, the same
gates apply (below).

## A working tree in four commands

You need **Go ≥ 1.27**, **Docker**, `golangci-lint`, and Node with pnpm
for the frontend half. *Which* pnpm is not yours to pick: `package.json`
pins it in `packageManager`, and both Corepack and pnpm itself switch to
that version, so the repository decides rather than the day you installed.

```
make install    # FE deps, gate tools, and the git hooks — run once
make db-up      # PG16 + Redis 7 containers, and the app role
make migrate    # core + custom migrations
make dev        # the whole stack: the app on :8080, the api behind it, worker
```

Two things about `make dev` that save an hour of confusion. It starts
**this worktree's** stack and leaves every other one alone, so a second
checkout can run at the same time. (`make dev-sweep` is the machine-wide
clear, and the only thing that touches somebody else's stack.) And the API
is **compiled**: Vite hot-reloads the frontend, the binary does not, so
every backend change needs `make dev-stop && make dev`. Not `make dev`
alone — this worktree's stack still holds the ports, and the boot refuses
them rather than talking over a server from an older build. A stale binary
is indistinguishable from a broken feature.

Full target list: [docs/reference/make-targets.md](docs/reference/make-targets.md).

## Branch, commit, merge

- **Branch off `main`**: `git switch -c <type>/<slug> origin/main`.
  Direct pushes to `main` are blocked; there is no other path to merge.
- **Conventional commit subjects**, scoped to the module:
  `fix(deals): a closed deal reports the stage it closed in`.
  Write the subject as the behaviour after the change, not as the task
  you performed.
- **Squash-merge** is the house style, and only over green checks.

### Contributing from a fork

A pull request from a fork runs the same gates and merges the same way as
a branch in this repository. If it is your first contribution here, the
checks wait until a maintainer approves the run, so expect a short delay
before CI reports. The SonarCloud analysis does not report on a fork,
because its token is withheld from fork-triggered workflows; it is not a
required check, so it does not block the merge.

## The gates

Every change — code, docs, and config alike — lands through the same
loop your PR will run:

1. **`make check`** is the merge gate: build, vet, lint (baseline +
   new-code strict), arch-lint, unit + fitness tests, generated-code
   drift, contract breaking-change, test-lane hygiene, image pins, and
   the file-length ratchet. It already includes the frontend lane
   (`check-fe`), so there is nothing to add on top. `make test-integration`
   is the separate real-Postgres lane — tenant isolation, GDPR erasure,
   audit immutability (needs `make db-up`). It fails loudly without a
   database rather than skipping, because a skipped security gate looks
   exactly like a passing one.
2. The **craftsmanship gate** (`craft static --strict`) runs
   diff-scoped on every push once hooks are installed (`make hooks`):
   new or touched backend code must be free of `BLOCKER` **and**
   `MAJOR` findings — a swallowed error, a sleep in a test, a bare
   `any` in a signature, a two-bool signature, an assertion-free test,
   or a function over 80 code lines (160 in `*_test.go`; a comment-only
   line is not counted, but a trailing comment does not make its code
   line free) all stop the
   push. `MINOR` is advisory. There is no grandfathered backlog: the
   tree was cleared to zero findings before this bar was armed, so
   `make craft-static` is green and CI runs the same bar as a required
   check. A *genuine* false positive is waived in-source
   with a reason: `//craft:ignore <check> <reason>` — that is the only
   way to stand a finding down, and the reason is read by the next
   contact to touch the line. The gate itself is a checksum-pinned
   binary that `scripts/craft-pin.sh` fetches on first use, so there is
   nothing to install and nothing in this repo to edit: the verdict you
   get on your laptop is the verdict your pull request gets.
3. **CI must be all green before merge**: the same deterministic gates
   plus automated review and static analysis. Address findings
   rather than dismissing them; squash-merge is the house style.

Write it right the first time: match the surrounding file, comments say
*why* not *what*, never swallow an error, and tests prove behaviour or
they are noise.

## Where things go

- Implementation decisions — anything the specification left open that
  the code had to decide — are explained in the commit message and PR
  description that makes the change; git history is the record.
- Open work lives in GitHub issues — there is no status file;
  start there, and read [AGENTS.md](AGENTS.md) for the binding
  engineering rules.
- Defects and proposals go to GitHub issues, which are templated: a bug,
  a **deferred follow-up** (anything you found but deliberately did not
  fix in the change at hand — that becomes an issue rather than a
  comment in the source), a capability gap or proposal, or a
  documentation defect. Security vulnerabilities go through
  [SECURITY.md](SECURITY.md) (private reporting), never a public issue.
- **This repository is public.** Nothing you write in an issue, a PR, or
  a commit may carry a secret, customer or personal data, a local
  machine path, or a private specification path or its contents — cite
  the spec by chapter, ADR, or pin ID instead.

Margince is built contract-first: `backend/api/crm.yaml` is the
authoritative surface. No separate specification outranks the running
software, its contract, its tests and its docs — if an older document
disagrees with the tree, name the conflict and keep going. Why it is
arranged that way:
[docs/principles/the-record-is-the-code.md](docs/principles/the-record-is-the-code.md).

## Before you open a PR

- Keep the PR scoped, and let it tell a story: what, why, and how it
  was verified.
- Run the pre-submit self-check in [AGENTS.md](AGENTS.md) →
  *Craftsmanship*.
- `make check` is green locally and every commit is signed off.
