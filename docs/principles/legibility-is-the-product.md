<!-- prose:plain -->
# Code a reader can follow

Code that reads best to a human reads best to the next agent that edits it. Whether a reader can
follow the code decides what the next change costs, and how safe it is.

The rule a change must follow is *Craftsmanship* in the rulebook. It has the anti-tell catalog
`T1`–`T11`, the rules to follow `P1`–`P5`, and the `craft static` gate. That gate gives the same
answer every time, looks only at the diff, and is strict. It checks the **Go files a push changes**;
a push of only docs stops before the gate runs at all. The rules are in that section of the
rulebook, and `craft rubric` prints the rubric the gate holds. What follows is the reason behind
them.

## Why this is a principle here and not only what a reviewer likes

Most readers of this code, human or agent, are not there when it is built, and know nothing past
the file. So the cost runs the other way. A short form that buys its author some time costs every
later reader the time to work it out. Each entry in the catalog is there because code without it
has already cost this tree a bug.

**Tell a machine reader the whole limit.** The `## Craftsmanship` section of the rulebook goes into
the gate prompt as it is. Take a rule that shows `fmt.Sprintf("%s = $%d", col, len(args))` and does
not say what may fill `%s`. A human reader works out "not a string from a request body". Every later
agent that reads it learns to build SQL injection through a column name.

## Where the rules need a reason

**Unit and integration tests fake other things.** In a unit test, a `time.Sleep`,
a real clock or a real network makes a test fail some of the time. The integration lane uses a real
Postgres and a real Redis. A stand-in on both sides of a line between two parts proves nothing about
that line. The integration lane fails without a database instead of skipping, so a
missing security gate cannot pass as a green one.

**The two size limits count lines differently.** Both measure what a reader must hold at once: 80
code lines per function and 500 per file for product code, 160 and 1000 for tests. The **function**
limit in `craft static` does not count a line that is only a comment, because an explanation makes
what a reader holds smaller. The whole-tree **file** limit in `scripts/check-go-file-length.sh` is
only a `wc -l` and counts every line.

A list file names each file that is over the limit and is older than the limit, and the list can
only get shorter. A long test of one case that sets up, does one thing and checks once is not a "god
function". A test file still splits when a reader cannot find their way in it.

## What this does not ask for

- **Not a question of how code looks.** `gofumpt` does that.
- **Not the same look in the whole tree.** The test is "no one can tell it from a senior human's
  edit to *this file*": match the file you are in.
- **Not a backlog of old findings.** The tree reached no findings before the gate started
  to block pushes. The rule is only that code you touch is clean.
