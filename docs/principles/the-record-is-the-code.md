<!-- prose:plain -->
# The record is the code

**The code, tests, migrations and `backend/api/crm.yaml` decide.** They say what this product does. A document about
them goes out of date, and nothing tells you. A test that goes out of date fails, so you see it,
*if* it still checks the obligation it is there to check. See
[derive the obligation](derive-the-obligation.md#writing-a-gate-that-holds) for the ways a green
test can prove nothing.

This principle is behind the order in `AGENTS.md` that decides a question. It is also the reason
that order puts the running software above every page of text but the current request and the
guardrails.

## Why the order is that order

1. **The current request, as someone states it**: what someone is asking to change.
2. **Code, tests, migrations, the contract**: what the product does *today*.
   These are the record itself.
3. **Guardrails**: security, privacy, what an agent may do, audit, a public contract that does not
   break, licensing, data that lasts. Tests hold them where they can, *and the test is the thing to
   read*. It states the obligation in a form that fails when the obligation stops holding.
4. **[docs/](../)**: how the product is built and run.
5. **Old pages kept only as history.** They never block work on their own.

The step that counts most is 2 above 4. A doc that disagrees with a green test is wrong about the
product. The fix is to change the doc, not to point to it as proof.

## How to use it

**When two sources disagree, run the code.** Run the test, read the migration, open the contract.
Do not trust one source over the other by how it reads. One of them is the record.

**A behaviour change goes into the record first**: a migration and a test, then the
doc about them. Do not merge a change to only docs that claims the product does something new.

**Each finding goes where it lasts long enough.** This is the rule that keeps the
record something a reader can use:

| What you find | Where it goes |
|---|---|
| A decision about how you built something | the commit and the PR: git history is the record |
| A decision that later work must follow | for the team to decide, where the reasons are kept |
| Something you find and do not fix here | a GitHub issue in this repository, with all three kinds of label, **unless an attacker can use it**. That goes to a private Security Advisory and never to a public issue ([nothing here is private](nothing-here-is-private.md)) |
| Open work, or where the next session starts | GitHub issues, labelled as [issue-labels.md](../reference/issue-labels.md) says |

**No notes about the build work in comments.** No review issue numbers, no story of the fix, no
"changed in answer to". State the invariant so it stands on its own. The story of how the code reached
this shape is in git. A comment that tells it a second time is a second copy that no one can query
and no one will keep up to date. The same goes for test names.

**Never excuse a known gap in a comment.** Change the code so the
gap is not there, or hold it with a test. A comment that says why something is wrong leaves it wrong.

## What this does not ask for

- **Not "documentation has no value".** `docs/` is step 4 of the order. It explains how the
  product is built and run, and that is work the code cannot do.
- **Not refusing change over an older document.** Name the conflict, say what it
  costs, and keep building. If the change touches a guardrail, say so in the PR, so the decision
  behind it changes with the code.
- **Not "tests are documentation".** A test states one obligation, and only that one. It does not
  guide a new reader. To use tests as the guide is how a tree becomes something only its writers can
  read.
