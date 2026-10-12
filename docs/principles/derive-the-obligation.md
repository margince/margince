<!-- prose:plain -->
# Derive the obligation

**Use a fitness function, not a point fix, and derive its set of subjects from the tree instead of
listing them.** A list kept by hand stops covering new subjects, and nothing fails.

The rule a change must follow is *Rules learned from the review loop* in the rulebook. This page is
how to apply it. See [reference/gate-patterns.md](../reference/gate-patterns.md) for the shapes
a gate comes in and how each one most often fails.
[reference/gate-inventory.md](../reference/gate-inventory.md) lists every gate.

## What deriving buys

- **A new subject is a finding** on the day someone writes it, and no one has to know that the
  gate exists. A list typed into a gate covers only the subjects its author could see when typing it.
- **You see the failure.** A rule kept by a comment fails when no one reads it, and that looks the
  same as a rule someone follows.
- **The odd case comes into the open.** A derived gate makes such a case get approved
  in writing, where a reviewer reads it, instead of going through with no sign.

## Writing a gate that holds

**Derive the set of subjects from the tree.** A map of what is covered, typed into a gate, is a list
to maintain, and it will be short. Say a gate checks that four places that draft text share the same
rules, while more than four exist. It is green, and it does not hold the rule.

**Put the way out at the subject**: a `doc.go` line, a contract field, a
`//craft:ignore <check> <reason>`. That is where the author who edits the code sees it. The reader
who needs a map in the test file never sees it. If you cannot get the way out to the subject, say
in the test why not.

**Approve the one case, never the whole kind.** A waiver keyed by column, package or rule name also
covers the second case for free, under the reason of the first.

**Write the test for the bug first**, and prove the mutation reached the code. Change the code to
put the bug back, and watch the gate go red. A gate that has never failed proves nothing: a fitness
function can pass against the bug it is about. A test of the case turned the other way can also pass
for the wrong reason. The mutation may go into a function the path never calls, or an earlier check
may refuse the case first. Check that each changed version still builds.

**Measure the before, not only the after.** A grep that returns 0 after the fix proves nothing unless
you know what it returned before.

**Watch for the filter that does nothing**, and the "some" that means nothing. A subject filter that
looks derived can do nothing at all. A filter that drops nothing reads the same as one that drops
the right things. The same problem in a reach gate is to ask whether *some* caller reaches the call
that must happen. Where that call has many call sites, that is true in most cases, and the gate
passes with the obligation removed. Measure what your gate leaves out.

**Write the mirror gate.** A gate can show green when its bug runs the other way. If you check that A
means B, ask what happens when B shows up without A.

**The gate is part of its own subject.** A gate that types in any part of what it holds has become a
second copy of it. The author who adds to the owner never sees a copy in a test.

One census of free
mail domains shipped with its own list of 25 domains. It kept that list in the test that exists to stop
a second list of free mail domains. Before the first check, ask what this gate types in: its file list, the things
it lists, the claim shapes it matches, its domains. Derive each from the owner, or write down why
you cannot.

**Add the case the gate cannot see.** One mutation proves the gate works; it does not find the hole.
Once the gate is green, ask which form of the bug its design cannot see, and add that case. Reading
the implementation one more time is not a good way to find the hole. What hides a copy from a reader
also hides it from the gate.

**Measure what a skip buys**, and remove a filter before you make it smaller. A
skip-list before a scan is where a gate stops seeing, and the miss gives no sign. The file is dropped
before the scan looks at it, so there is no finding, and the result looks like a tree with no
problems. A skip-list you have to keep making smaller must be removed instead. To parse every file
often takes no more time than the skip.

**Match statements, not lines, and limit the join.** A matcher that reads one line at a time misses a
rule that a tool split over three lines. Joining lines fixes that and adds a new failure. A join with
no limit can take in a whole `const (` block. It can then report two names in it that have nothing
to do with each other.

**Check what the gate said**, not only that it failed. A scanner that reports both the waived and the
not-waived finding fails too.

**Leave the language rules to a parser.** A gate that matches text gets bugs in the matcher instead
of in the rule. Some examples: a missing `\b`, or a built-in name read as `-inf`. Or a `\` whose meaning in Go
back quotes is not its meaning in TypeScript back quotes. A parser that already knows the
language answers these for free: `go/ast` in Go, `ts.createSourceFile` in TypeScript. A parse error is
also not silence, which is the failure a text gate must add a check to see.

## Find the other side before you fix this one

Most things here are built once, and the other language only shows them. Go and TypeScript may each
have a copy of the same rule. The two are then **one thing to fix** until you prove they are not, and
a PR for each language hides that.

An example: a frontend writes `Math.round(amount * 100)` for every currency, and a backend turns it
back with `amount / 100` for every currency. The two errors **hide** each other. A price in a currency
that has no smaller unit is stored at 100 times its value, and the screen shows it right. So the screen agrees with
itself, and only the record is wrong. To fix only the backend side puts 100 times the price on an
outbound offer.

So: merge both sides in one change, then declare which side is the **mirror**, and gate it both ways.
`backend/gates/frontendminorunits_test.go` is the worked example. It fails on a currency that is in
one side and not the other. It also fails on a currency whose unit size the two sides disagree on. The one
thing kept in one place there is the table the two sides share; the test files keep their own cases.

Refuse to split one rule into "a Go test for Go and a text scan for TypeScript". That leaves one rule
with two implementations, and nothing makes you change them together. The two sides may use other
parsers, but never other rules.

## What this does not ask for

- **Not a gate for every rule.** Some obligations need a human to decide, and a gate that gets the
  call wrong does more harm than the rule in text. If you cannot state the failure in a way a test can
  check, write the rule and say no gate holds it.
- **Not blocking on tools.** A point fix that ships today, plus an issue for the gate, does more good
  than a gate that never ships. Say which one it is.
- **Not removing a rule** because it has no gate. Nothing holds a rule with no gate but you, and
  you must still follow it.
