# Derive the obligation

**Prefer a fitness function over a point fix, and derive its subject set from
the tree rather than listing it.** A rule maintained as a list is a rule that
silently stops covering things.

The binding form is *Rules learned from the review loop* in the rulebook. This
page is the method. The eight shapes a gate comes in, and how each one
characteristically fails, are cataloged in
[reference/gate-patterns.md](../reference/gate-patterns.md); the gates
themselves are listed in
[reference/gate-inventory.md](../reference/gate-inventory.md).

## What deriving buys

- **A new subject is a finding on the day it is written**, without anybody
  remembering the gate exists. A hardcoded list covers what somebody knew about
  once.
- **The failure is loud.** A rule held by a comment fails by being ignored,
  which looks exactly like a rule being followed.
- **The exception becomes visible.** A derived gate forces an unusual case to be
  ratified in writing, where a reviewer reads it, instead of being absorbed
  silently.

## Writing a gate that holds

Every clause here exists because a gate in this tree failed it.

**Derive the subject set from the tree.** A hardcoded coverage map is a list to
maintain, and it will be short. One parity gate asserts four drafting surfaces
carry the shared rules while more than four exist: green, and the rule is not
held.

**Put the escape hatch at the subject** — a `doc.go` line, a contract field, a
`//craft:ignore <check> <reason>`. That is where the author editing the code
sees it. A map inside the test file is invisible to exactly the person who needs
it. If you cannot get the hatch to the subject, say in the test why not.

**Ratify the instance, never the category.** A waiver keyed by column, package
or rule name admits the second offender for free under the first one's reason.

**Write the defect test first, and prove the mutation was reached.** Mutate the
code to reintroduce the bug and watch the gate go red. A gate that has never
failed proves nothing — several fitness functions here passed against the exact
bug they described. An inverse test can also pass for the wrong reason: the
mutation lands in a function the path never calls, or an earlier guard refuses
the case first. Check that each mutant compiles.

**Measure the before, not only the after.** A grep returning 0 after the fix
proves nothing unless you know what it returned before.

**Watch for the vacuous filter, and for the vacuous quantifier.** A subject
filter that looks derived can be inert — a predicate that drops nothing reads
exactly like one that drops the right things. The same trap in a reachability
gate is asking whether *some* caller reaches the required call: where that call
has several call sites, it is true of nearly everything, and the gate passes
with the obligation deleted. Measure what your gate excludes.

**Write the mirror gate.** A gate can read green over its own defect reversed.
If you assert A implies B, ask what happens when B appears without A.

**The gate is part of its own subject.** A gate that hardcodes any slice of what
it guards has become a second copy of it, and a copy inside a test is the worst
kind — invisible to the author extending the owner. A consumer-mail census
shipped with its own 25-domain sample inside the very test that forbids second
consumer-mail lists. Before the first assertion, ask what this gate hardcodes:
its file list, its element list, its claim patterns, its domains. Derive each
from the owner, or write down why you cannot.

**Plant the shape the detector cannot see.** One mutation proves the gate fires;
it does not find the hole. Once it is green, ask what version of the defect the
gate is structurally blind to, and plant that case. Every hole found during the
duplication sweep was found this way, and none by re-reading the implementation
— because what hid a copy from a reader also hid it from the detector.

**Measure a shortcut before you defend it, and prefer deleting a dimension to
narrowing it.** A skip-list in front of a scan is where a gate goes blind, and
the miss is silent: the file is dropped before anything looks at it — no
finding, indistinguishable from a clean tree. One census carried a file-skip
that six review rounds found too narrow in six separate dimensions. It was
deleted rather than narrowed a seventh time, and parsing every file turned out
to be faster than the shortcut.

**Match statements, not lines, and bound the join.** A per-line matcher misses a
rule a formatter wrapped across three lines. Joining lines fixes that and
introduces the opposite failure: unbounded, one join swallowed a thirty-line
`const (` block and reported an unrelated pairing inside it.

**Assert on what the gate said, not on its exit status.** A scanner that reports
both the waived and the unwaived finding exits non-zero too.

**Let a parser own the grammar.** One text-matching gate produced six defect
classes in a single PR, every one a defect in the matcher rather than in the
rule — a missing word boundary, a built-in name coerced to `-inf`, a backslash
meaning different things in a Go raw string and a TS template. A parser that
already knows the language answers all six for free: `go/ast` in Go,
`ts.createSourceFile` in TypeScript. A parse error is also not silence, which is
the failure mode a text gate must invent an assertion to catch.

## Find the other side before you fix this one

Most topics here are implemented once and merely rendered by the other language.
This is about the ones where Go and TypeScript each carry a spelling of the same
rule: those are **one item** until proven otherwise, and a per-language PR hides
that.

The case that taught it: the frontend wrote `Math.round(amount * 100)` for every
currency and the backend divided by 100 for every currency, so the two errors
**cancelled**. A zero-decimal price was stored a hundredfold and displayed
correctly — the screen agreed with itself, and only the record was wrong.
Shipping the backend half alone would have uncancelled them and printed a
hundred times the price on an outbound offer.

So: land both sides in one change, then declare which side is the **mirror** and
gate it in both directions. `backend/gates/frontendminorunits_test.go` is the worked
example — it fails on a currency present in one side and not the other, and on a
digit count that differs. What stays singular there is the table the two sides
exchange; the suites keep their own cases.

The move to refuse is splitting one rule into "a Go test for Go and a text scan
for TypeScript". That leaves one rule with two implementations that nothing
forces to be edited together. What may differ between the two sides is the
parser; never the rule.

## What this does not ask for

- **Not a gate for every rule.** Some obligations are genuinely judgement, and a
  gate that encodes a judgement badly is worse than the prose. If you cannot
  state the failure precisely, write the rule and say it is unguarded.
- **Not blocking on tooling.** A point fix that ships today plus an issue for
  the gate beats a gate that never lands. Say which you did.
- **Not deleting a rule because it has no gate.** An unguarded rule is weaker,
  not void.
