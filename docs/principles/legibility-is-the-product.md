# Legibility

Code that reads best to a human reads best to the next agent that edits it.
Legibility decides whether the next change is cheap or dangerous.

The binding form is *Craftsmanship* in the rulebook: the anti-tell catalog
T1–T11, the positive rules P1–P5, and the deterministic `craft static` gate,
diff-scoped and strict. It judges the **Go files a push changes**; a docs-only
push exits before it runs at all. The rules themselves are in that section of
the rulebook, and `craft rubric` prints the rubric the gate carries. What
follows is the reasoning under them.

## Why this repo takes it as a principle rather than taste

Most of this codebase is read by someone, or something, that was not present
when it was written and holds no context beyond the file. Under that constraint
the usual trade-off inverts: a clever compression that saves a human author ten
minutes costs every later reader the reconstruction. Each entry in the catalog
is there because its absence has already cost this tree a defect.

**A machine reader needs the caveat spelled out.** The
rulebook's `## Craftsmanship` section goes into the gate prompt as written. A
rule showing `fmt.Sprintf("%s = $%d", col, len(args))` without saying what may
fill `%s` leaves a human to infer "not a request-body string", but teaches
identifier injection to every future agent that reads it.

## Where the rules need a reason

**Unit tests and integration tests fake different things.** In a unit test, a
`time.Sleep`, a real clock or a real network is a source of flakiness. The
integration lane uses a real Postgres and a real Redis, because a boundary faked
on both sides proves nothing about the boundary. The integration lane fails
without a database instead of skipping, so that a missing security gate cannot
pass as a green one.

**The two size ceilings count differently.** Both measure what a reader must hold
at once: 80 code lines / 500 file lines for product code, 160 / 1000 for tests.
`craft static`'s **function** ceiling discounts a comment-only line, because an
explanation reduces what a reader carries. The whole-tree **file** cap in
`scripts/check-go-file-length.sh` is a plain `wc -l` and counts every line, with
a ratchet file freezing each pre-existing offender. A long scenario test that
sets up, acts and asserts once is not the god-function smell; a suite still
splits when it stops being navigable.

## What this does not ask for

- **Not a style debate.** Formatting is `gofumpt`'s job.
- **Not uniformity across the tree.** The bar is "indistinguishable from a senior
  human's edit to *the surrounding file*": match the file you are in.
- **Not a backlog to work through.** The tree was cleared to zero findings
  before the gate was armed. The rule is only that touched code is clean.
