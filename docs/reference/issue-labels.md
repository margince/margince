<!-- prose:plain -->
# Issue labels

Every issue in this repository carries one `priority:` and one `area:`. It also carries a `status:`
when it is not work for now or someone already has it, and each source label that fits. This page
is the full list; the short rule is in `AGENTS.md`.

The label set itself lives in [`.github/labels.yml`](../../.github/labels.yml). That file is the
source, not another copy. `scripts/sync-labels.sh` sets the labels of the repository from it, and
`backend/gates/issuelabels_test.go` fails `make check` if a section below and that file stop naming
the same set. Change the file and this page together: a new label is one edit to the file and one to
the section it belongs in.

## What a missing label tells the reader

The labels keep one rule true: no labels means nobody has looked at the issue yet. So a reader can
scan the issue list and tell work someone has sorted from work nobody has sorted, at once. An issue
filed without labels looks as if nobody has judged it, when you just did. So it tells the
next reader something false about your finding.

## Priority

**Priority** says how serious the problem is; the milestone holds the schedule. Do not lower a real
bug because it is not work for this week. Use the milestone, or `status: deferred` when no
milestone fits it yet:

| Label | It fits when |
|---|---|
| `priority: critical` | Data loss, a security or privacy hole someone can reach, `main`/CI red, or the product does not work on a default install. Drop other work. |
| `priority: high` | A real user or operator hits it on a live path, or it blocks another line of work. |
| `priority: normal` | A real bug that is narrow, guarded, or out of reach today; work that keeps the code in good shape; test lane work; small fixes. |
| `priority: low` | A wish, not a bug, or it needs a product decision before it is work at all. |

Use `critical` when the issue stops others from doing any work, not only one line of it. Use it too
when the data of someone is wrong right now. A gate that fails only some of the time fits: while nobody can trust a red run, nobody can read any other
result.

## Area

**Area** is where the fix lives, one only, so a filter never counts an issue twice. The areas are:
`agents-mcp` · `ai-models` · `authz` · `capture` · `ci-tests` · `contract-api` ·
`deals` · `extensions` · `finance` · `frontend` · `platform` ·
`privacy` · `records` · `reports`. A doc that is wrong about a part of the system takes the area of that part. That way it sits next
to the code it is wrong about. There is no documentation area.

## Status

**Status**, when it applies, marks an issue nobody *else* should pick up. Nobody can work on
it, or it is not work for now by agreement, or someone already has it. To leave it off puts the
issue in the queue of someone:

- `status: in progress`: someone is working on it right now. It goes on together with the
  assignee when the work starts. It comes off when the last assignee leaves or the issue closes.

  `issue-closed.yml` removes it on close. It does so at once. A failed run, or a close that no event reports (such as one a
  workflow makes), waits for the next daily check. One label serves all who hold the issue, so a
  session that drops itself as assignee does not take the label with it.

  Both are needed. The assignee is what a reader sees in the issue list, and the label is
  what a search can leave out. A comment that says `I am on this` does not do either. To check before you
  start, claim an issue, take one over, and let one go:
  [../how-to/work-on-an-issue.md](../how-to/work-on-an-issue.md).
- `status: needs-decision`: nobody can work on it until a human decides, whether the call is technical or
  a product call. Say what the options are and which one you think is best. An issue that only
  asks "what should we do?" gives the human who decides nothing to decide from.
- `status: deferred`: we know the work and agree on it, and it is not on the schedule: a later release, and nobody
  knows which one yet. It keeps its real priority, because to defer sets a schedule, and the
  priority says how serious it is. To mark a real bug down to `priority: low` makes the issue
  list wrong about what is wrong with the product. Drop the label when the work gets a milestone.

  Say in the issue what it waits for, so the reader who filters it back in knows what changed.

## Claim

This is the one kind of label that goes on a **pull request**, not an issue. It says who is
working, as `status: in progress` does. But a pull request carries it, and a red `main` has no
issue to label.

- `claim: main-red`: a session is already fixing this red on `main`. It sits on a draft pull request
  whose body lists the failing lanes and tests it covers. The list matters because `main` is often
  red for two reasons at once that have nothing to do with each other. A claim that names one of
  them leaves the other free for someone to take. Check for a claim before you look into a failure
  you did not cause. If no claim covers yours, open one before you start a fix.

  The steps, and
  when someone may take over an old claim, are in
  [../how-to/claim-a-red-main.md](../how-to/claim-a-red-main.md).

  It is a `claim:` label because a status sits on an issue. The thing claimed here is a break on
  `main` that no issue names.

## Provenance

**Provenance** labels say where an issue comes from. They add to priority, area and status, and
are separate from them. They are:

- `bug` and `enhancement`.
- `security` (see below).
- `capability-gap`: a missing feature, not a bug.
- `fast-track-debt`: shipped fast under time limits, with the gap on record.
- `margince-qc`: found by the `margince-qc` UAT test repository while someone built or ran a
  test case there, not by someone who works in this repository.
- `schema-review`: found by reading the database table by table. The fix is a migration or the
  contract a column says it keeps, and no one reported a problem in use.

These record *why the issue is there*, which nobody can work out later, so keep them; do not
remove them.

**`security` does not report a security hole**. This repository is public.
[SECURITY.md](../../SECURITY.md) sends a hole someone can use to a private GitHub Security
Advisory, never a public issue or pull request. A public report before a fix ships puts every
install at risk.

The label is for work that makes the code harder to attack, where no live attack
works. The test comes from `SECURITY.md`: if you can write the steps that make the attack work, it
belongs in an advisory. A hole you think exists but cannot yet show also goes to an advisory, not to a public issue. Some cases:

- a read from one tenant into another;
- a way out of row scope or RBAC;
- a way past agent rules;
- a forged credential, or a revoked one that still works;
- a write that skips the audit or outbox row;
- injection, SSRF.

## Before you file: look for a tracker

Run `gh issue list --label "area: <x>"` and look for a tracker that already covers the finding. If
one is there, add yours as a sub-issue, not one more issue next to the rest. A tracker carries the
highest priority of any sub-issue under it, so a sub-issue at `critical` raises the tracker.
