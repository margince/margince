# Issue labels

Every issue in this repository carries **exactly one `priority:` and exactly one
`area:`**, plus a `status:` when it is not yet workable and whatever provenance
labels apply. This page is the full taxonomy; the binding short form is in
`AGENTS.md`.

The label set itself lives in [`.github/labels.yml`](../../.github/labels.yml),
which is the source rather than another copy: `scripts/sync-labels.sh`
reconciles the repository's own labels from it, and
`backend/gates/issuelabels_test.go` fails `make check` if a section below and
that file stop naming the same set. So editing this page alone changes nothing,
and editing the file alone fails the gate — which is the point. Adding a label
is one edit to the file and one to the section it belongs in.

## Why an unlabeled issue is worse than no issue

The labels protect one invariant: **unlabeled means nobody has looked at it yet.**
That is what lets anyone scan the tracker and tell triaged work from untriaged
work at a glance. File an issue without labels and you have quietly told the next
reader something false about your own finding — it looks like nobody has assessed
it, when in fact you just did.

Every issue you open carries **exactly one `priority:` and exactly one `area:`**.

## Priority

**Priority** is a claim about severity, never about your schedule. Do not demote
a real defect because it is not this week's work — the milestone carries the
schedule, the label carries the truth:

| Label | It qualifies when |
|---|---|
| `priority: critical` | Data loss, a reachable security or privacy breach, `main`/CI red, or the product unusable on a default install. Drop other work. |
| `priority: high` | A real user or operator hits it on a live path, or it blocks another workstream. |
| `priority: normal` | A genuine defect that is narrow, guarded, or unreachable today; hygiene; test-lane work; polish. |
| `priority: low` | A want, not a defect — or it needs a product decision before it is work at all. |

The honest test for `critical` is not "how bad would this be" but "does this stop
somebody else from working, or is somebody's data wrong right now". A flaky gate
earns it not because it is severe but because a red run nobody trusts makes every
other verdict unreadable.

## Area

**Area** is where the fix lives, one only, so a filter never double-counts:
`agents-mcp` · `ai-models` · `authz` · `capture` · `ci-tests` · `contract-api` ·
`deals` · `extensions` · `finance` · `frontend` · `overlay` · `platform` ·
`privacy` · `records` · `reports`. A doc that is wrong about a subsystem takes
that subsystem's area, not a documentation area — it belongs next to the code it
misleads about.

## Status

**Status**, when it applies — these mark an issue that is not yet workable, and
leaving them off puts unactionable work in somebody's queue:

- `status: needs-decision` — unactionable until a human rules, whether the ruling
  is technical or a product call. Say what the options are and which you
  recommend: an issue that only asks "what should we do?" gives the decider
  nothing to decide from.

## Provenance

**Provenance**, additive and independent of the three axes: `bug`,
`enhancement`, `security`, `capability-gap` (a missing capability, not a defect),
`fast-track-debt` (shipped fast under time pressure with the gap recorded
deliberately), and `margince-qc` (found by the `margince-qc` UAT acceptance-test
repo while building or running a scenario, rather than by a person working in
this repo directly). These record *why the issue exists*, which is the one
thing nobody can reconstruct later — prefer keeping them over tidying them
away.

**`security` is not a way to report a vulnerability.** This repo is public, and
[SECURITY.md](../../SECURITY.md) is explicit that an exploitable weakness goes to a
private GitHub Security Advisory, never a public issue or pull request, because
a public report before a fix ships puts every deployment at risk. The label is
for hardening and defence-in-depth work that carries no live exploit. The test
is the one SECURITY.md itself implies: **if you can write the reproduction, it
belongs in an advisory** — a cross-tenant read, a row-scope or RBAC escape, an
agent-governance bypass, a forged or still-binding revoked credential, a
mutation that skips the audit or outbox row, injection, SSRF.

## Before you file: check for a parent

Run `gh issue list --label "area: <x>"` and look for a tracker that already
covers the finding. If one exists, attach yours as a sub-issue rather than adding
another sibling to the pile. A tracker carries the highest priority among its
children, so a critical child raises the parent.
