# Issue labels

Every issue in this repository carries one `priority:` and one `area:`, plus a
`status:` when it is not now's work or is already somebody's, and whatever
provenance labels apply. This page is the full taxonomy; the binding short form
is in `AGENTS.md`.

The label set itself lives in [`.github/labels.yml`](../../.github/labels.yml),
which is the source rather than another copy: `scripts/sync-labels.sh`
reconciles the repository's own labels from it, and
`backend/gates/issuelabels_test.go` fails `make check` if a section below and
that file stop naming the same set. Edit the file and this page together: adding
a label is one edit to the file and one to the section it belongs in.

## What a missing label tells the reader

The labels protect one invariant: unlabelled means nobody has looked at it yet.
That lets anyone scan the tracker and tell triaged work from untriaged work at a
glance. An issue filed without labels looks unassessed, although you just
assessed it, so it tells the next reader something false about your finding.

## Priority

**Priority** is a claim about severity; the milestone carries the schedule. Do
not demote a real defect because it is not this week's work. Use the milestone,
or `status: deferred` when no milestone fits it yet:

| Label | It qualifies when |
|---|---|
| `priority: critical` | Data loss, a reachable security or privacy breach, `main`/CI red, or the product unusable on a default install. Drop other work. |
| `priority: high` | A real user or operator hits it on a live path, or it blocks another workstream. |
| `priority: normal` | A real defect that is narrow, guarded, or unreachable today; hygiene; test-lane work; polish. |
| `priority: low` | A want rather than a defect, or it needs a product decision before it is work at all. |

Use `critical` when the issue stops somebody else from working, or somebody's
data is wrong right now. A flaky gate qualifies: while a red run is untrusted,
nobody can read any other verdict.

## Area

**Area** is where the fix lives, one only, so a filter never double-counts:
`agents-mcp` · `ai-models` · `authz` · `capture` · `ci-tests` · `contract-api` ·
`deals` · `extensions` · `finance` · `frontend` · `platform` ·
`privacy` · `records` · `reports`. A doc that is wrong about a subsystem takes
that subsystem's area, so it sits next to the code it misleads about. There is
no documentation area.

## Status

**Status**, when it applies, marks an issue nobody *else* should pick up: it
cannot be worked, it is not now's work by agreement, or somebody already has it.
Leaving one off puts that issue in somebody's queue:

- `status: in progress`: somebody is working on it right now. It goes on
  together with the assignee when the work starts, and comes off when the last
  assignee leaves or the issue closes. `issue-closed.yml` removes it on close:
  at once, or by the next daily sweep for a close no event reports (a
  workflow's own) or a failed run. One label serves however many hold the
  issue, so a session dropping its own assignment does not take it with them.
  Both signals are needed: the assignee is what a reader sees in the issue list,
  the label is what a search can exclude, and a comment saying "I am on this"
  is neither.
  Checking before you start, claiming, taking one over and releasing it:
  [../how-to/work-on-an-issue.md](../how-to/work-on-an-issue.md).
- `status: needs-decision`: unactionable until a human rules, whether the ruling
  is technical or a product call. Say what the options are and which you
  recommend; an issue that only asks "what should we do?" gives the decider
  nothing to decide from.
- `status: deferred`: understood and agreed, and not scheduled: a later
  release, and nobody knows which yet. It keeps its real priority, because
  deferring is a schedule and the priority is the severity. Parking a real
  defect by relabelling it `priority: low` makes the tracker misstate what is
  wrong with the product. Drop the label when the work gets a milestone. Say in
  the issue what it is waiting for, so the reader who filters it back in knows
  what changed.

## Claim

The one axis that goes on a **pull request** rather than an issue. It says who
is working, as `status: in progress` does; the difference is what wears it, and
a red `main` has no issue to label.

- `claim: main-red`: a session is already fixing this red on `main`. It rides a
  draft pull request whose body lists the failing lanes and tests it covers.
  The list matters because `main` is regularly red for two unrelated reasons at
  once, so a claim naming one of them leaves the other unclaimed and free to
  take. Check for one before you investigate a failure you did not cause, and
  open one before you start fixing if none covers yours. The procedure, and when
  a stale claim may be taken over, is
  [../how-to/claim-a-red-main.md](../how-to/claim-a-red-main.md).

  It is a `claim:` label because a status rides the issue, and the thing being
  claimed here is a break on `main` that no issue names.

## Provenance

**Provenance** labels are additive, and independent of priority, area and status.
They are:

- `bug` and `enhancement`.
- `security` (see below).
- `capability-gap`: a missing capability rather than a defect.
- `fast-track-debt`: shipped fast under time pressure, with the gap recorded.
- `margince-qc`: found by the `margince-qc` UAT acceptance-test repo while
  building or running a scenario, rather than by someone working in this repo
  directly.
- `schema-review`: found by reading the database table by table, so the fix is a
  migration or the contract a column claims to keep, where no one reported a
  behaviour.

These record *why the issue exists*, which nobody
can reconstruct later, so keep them rather than tidying them away.

**`security` does not report a vulnerability**. This repo is public.
[SECURITY.md](../../SECURITY.md) sends an exploitable weakness to a private
GitHub Security Advisory, never a public issue or pull request, because a public
report before a fix ships puts every deployment at risk. The label is
for hardening and defence-in-depth work that carries no live exploit. The test
follows from SECURITY.md: if you can write the reproduction, it belongs in an
advisory. Examples: a cross-tenant read, a row-scope or RBAC escape, an
agent-governance bypass, a forged or still-binding revoked credential, a
mutation that skips the audit or outbox row, injection, SSRF.

## Before you file: check for a parent

Run `gh issue list --label "area: <x>"` and look for a tracker that already
covers the finding. If one exists, attach yours as a sub-issue rather than adding
another sibling to the pile. A tracker carries the highest priority among its
children, so a critical child raises the parent.
