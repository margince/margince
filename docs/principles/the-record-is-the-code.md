# The record is the code

**Code, tests, migrations and `backend/api/crm.yaml` define what this product
does.** A document describing them goes stale silently. A test goes stale
loudly, *if* it still exercises the obligation it was written for; see
[derive the obligation](derive-the-obligation.md#writing-a-gate-that-holds)
for the ways a green one can prove nothing.

This is the principle behind the precedence order in `AGENTS.md`, and the reason
that order puts the running software above every prose artefact except the
current request and the guardrails.

## Why the order is that order

1. **The explicit current request**: what someone is asking to change.
2. **Code, tests, migrations, the contract**: what the product does *today*.
   These are the record itself.
3. **Guardrails**: security, privacy, agent authority, auditability, contract
   compatibility, licensing, durability. Enforced by tests wherever possible,
   *and the test is the thing to read*: it states the obligation in a form that
   fails when the obligation stops holding.
4. **[docs/](../)**: how the product is built and operated.
5. **Retired material**: history. It never blocks work on its own.

The step that matters most is 2 above 4. A doc that disagrees with a green test
is wrong about the product, and the fix is to change the doc rather than argue
from it.

## The method

**When two sources disagree, run the code.** Execute the test, read the
migration, open the contract, instead of deciding which source sounds more
authoritative. One of them is the record.

**A behaviour change lands in the record first**: a migration
and a test, then the doc that describes them. Do not land a doc-only change that
claims new behaviour.

**Findings route by durability**, and this is the rule that keeps the record
readable:

| The thing you found | Where it goes |
|---|---|
| An implementation decision you made | the commit and the PR — git history is the record |
| A decision that binds future work | raised with the team, where the reasoning is kept |
| Something found but not fixed here | a GitHub issue in this repo, labelled on all three axes, **unless it is an exploitable weakness**, which goes to a private Security Advisory and never a public issue ([nothing here is private](nothing-here-is-private.md)) |
| Open work / the session pickup point | GitHub issues, labelled per [issue-labels.md](../reference/issue-labels.md) |

**No build-process residue in comments.** No review-ticket numbers, no fix
narration, no "changed in response to". State the invariant so it stands alone.
The story of how the code got this way is in git; a comment retelling it is a
second copy that cannot be queried and will not be updated. Same for test
names.

**Never rationalize a known gap in a comment.** Restructure it away or gate it
with a test. A comment explaining why something is broken leaves it broken.

## What this does not ask for

- **Not "documentation does not matter".** `docs/` is step 4 of the order. It
  explains how the product is built and operated, and that is work the code
  cannot do.
- **Not refusing change because an older document disagrees.** Name the
  conflict, say what it costs, and keep building. If the change touches a
  guardrail, say so in the PR so the decision behind it is updated with the
  code.
- **Not "tests are documentation".** A test states one obligation precisely. It
  does not orient a newcomer, and pretending it does is how a tree becomes
  unreadable to everyone who did not write it.
