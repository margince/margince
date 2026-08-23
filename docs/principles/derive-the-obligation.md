# Derive the obligation

**Prefer a fitness function over a point fix, and derive its subject set from
the tree rather than listing it.** A rule maintained as a list is a rule that
silently stops covering things.

The binding form is
[*Rules learned from the review loop*](../../CLAUDE.md#rules-learned-from-the-review-loop-binding).
This page is the method for writing a gate that actually holds.

## The eight rules, and what each one is defending against

1. **Fix the invariant, not the call site.** Grep every mutation and read site
   of the same column, constraint or record, and fix them as one change. The
   recurring reviewer catch here is "fixed the case under review, missed the
   sibling copy".
2. **Prefer fitness functions over point fixes.** Derive the obligation from the
   system — every tenant statement carries its workspace predicate; every CHECK
   violation maps to a 4xx; `backend/arch_test.go` derives its package lists
   from the tree. Do not maintain it as a list.
3. **Anything that returns a record is a read**, and carries the row-scope gate
   — including replay, conflict and error paths.
4. **No build-process residue in comments.** State the invariant so it stands
   alone. See [the record is the code](the-record-is-the-code.md).
5. **Never rationalize a known gap in a comment.** Restructure it away or gate
   it.
6. **A test that supplies its own version of production proves nothing about
   production.** Hand-inserted rows the real writer never writes, or a
   hand-copied adapter mirroring what compose wires. Seed through the real
   writer; reach for the real wiring. An unexpectedly uncovered new file usually
   means a test double stands where the real thing should.
7. **One invariant broken in two languages is one item.** Fixing one side of a
   wire alone can be a regression, not half a fix. See
   [find the other side](#find-the-other-side-before-you-fix-this-one).
8. **A census that can fail short has already failed.** Under-recognition is
   silent: the gate reads less and still says PASS. Everything under *Writing a
   gate that holds* below is about this one.

## Writing a gate that holds

Every clause here exists because a gate in this tree failed it.

**Derive the subject set from the tree.** A hardcoded coverage map is a list to
maintain, and it will be short. `draftrulesparity_test.go` asserts four drafting
surfaces carry the shared rules while more than four exist — the gate is green
and the rule is not held.

**Put the escape hatch at the subject.** A `doc.go` line, a contract field, a
`//craft:ignore <check> <reason>` — where the author editing that code sees it.
A map inside the test file is invisible to exactly the person who needs it.
`agenttoolparity_test.go` is the closest thing to the house shape: it derives
its expectation from the contract rather than restating it, and its escape hatch
for ordinary tools lives in the contract. It is not a pure example, and the
impurity is instructive — a hand-maintained `composedIntents` map still sits in
the test file for the tools that compose several operations, which is precisely
the kind of list that goes quietly short. If you cannot get the hatch to the
subject, say in the test why not.

**Ratify the instance, never the category.** A waiver keyed by column, package
or rule name admits the second offender for free under the first one's reason.

**Write the defect test first.** Mutate the code to reintroduce the bug and
watch the gate go red. A gate that has never failed proves nothing — several
fitness functions in this tree passed against the exact bug they described.

**Prove the mutation was reached.** An inverse test can pass for the wrong
reason: the mutation lands in a function the path never calls, or an earlier
guard refuses the case before your assertion runs.

**Measure the before, not only the after.** A grep returning 0 after the fix
proves nothing unless you know what it returned before.

**Watch for the vacuous filter.** A subject filter that looks derived can be
inert — unbounded is not empty, and a predicate that drops nothing reads exactly
like a predicate that drops the right things. Measure what your gate *excludes*.

**Write the mirror gate.** A gate can read green over its own defect reversed.
If you assert A implies B, ask what happens when B appears without A.

**The gate is part of its own subject.** A gate that hard-codes any slice of
what it guards has become a second copy of it — and the copy inside a test is
the worst kind, because it is invisible to the author extending the owner. A
consumer-mail census shipped with its own 25-domain sample inside the very test
that forbids second consumer-mail lists; two reviewers named it independently,
and it asks `freemail.Domains()` now. Before the first assertion, ask what this
gate hard-codes: its file list, its element list, its claim patterns, its
domains. Derive each from the owner, or write down in the test why you cannot.

**Plant the shape the detector cannot see.** One mutation proves the gate fires;
it does not find the hole. Once the gate is green, ask what shape of the defect
it is structurally blind to and plant that case. Every hole found in review
during the duplication sweep was found this way, and none by re-reading the
implementation — because the thing that hid a copy from a reader also hid it
from the detector written to find copies. A `String(err)` hid behind being the
fallback half of a ternary somebody had already fixed; a raw backtick string hid
behind a prefilter that only knew double quotes.

**Measure a shortcut before you defend it, and prefer deleting a dimension to
narrowing it.** A skip-list or prefilter in front of a scan is where the gate
goes blind, and the miss is silent: the file is dropped before anything looks at
it — no finding, no error, indistinguishable from a clean tree. One census in
this tree carried a cheap file-skip that six review rounds found narrower than
the census behind it in six separate dimensions, two of them introduced by the
fix for the one before. It was deleted rather than narrowed a seventh time,
because measuring ended the argument: parsing every file was FASTER than the
shortcut, and the saving originally credited to it had come from an unrelated
map lookup.

**Statements, not lines, and bound the join.** A per-line matcher misses a rule
a formatter wrapped across three lines — the money gate missed an entire write
direction that way. Joining lines into statements fixes it and introduces the
opposite failure: unbounded, one join swallowed a thirty-line `const (` block
and reported an unrelated pairing inside it.

**A self-test that reads only an exit status proves nothing about a waiver.** A
scanner that reports both the waived and the unwaived finding exits non-zero
too. Assert on what the gate SAID, not just that it failed.

**Let a parser own the grammar.** One text-matching gate produced six defect
classes in a single PR, every one of them a defect in the matcher rather than in
the rule: no `\b` in POSIX ERE so a guard shipped inert, a built-in name
coerced to `-inf`, an undeclared local colliding as a global, a flat flag unable
to express a nested template literal, a backslash meaning different things in a
Go raw string and a TS template. A parser that already knows the language
answers all six for nothing — `go/ast` in Go, `ts.createSourceFile` in
TypeScript, both already used by censuses in this tree. And a parse error is not
silence, which is the failure mode a text gate has to invent an assertion to
catch.

## Find the other side before you fix this one

When the same invariant is spelled in Go and in TypeScript, it is one item until
proven otherwise, and a per-language PR is what hides that.

The case that taught it: the frontend wrote `Math.round(amount * 100)` for every
currency and the backend divided by 100 for every currency, so the two errors
CANCELLED — a zero-decimal price was stored a hundredfold and displayed
correctly, the screen agreed with itself, and only the record was wrong. The
sweep catalogued the two halves as two findings on two tiers. Shipping the
backend half alone would have uncancelled them and printed a hundred times the
price on an outbound offer.

So: when a sweep finds one invariant broken on both sides of a wire, land both
sides in one change, and give the two test suites **one corpus** — a single
case file tagged by language, read by both, with a drift gate over it
(`frontend/src/format/minorunits.ts` + `backend/frontendminorunits_test.go`).
What stays singular is the rule and the case corpus; only the parser differs,
and neither parser is hand-written. Splitting a rule into "a Go test for Go and
a text scan for TypeScript" is the defect wearing the fix's clothes.

## What this does not ask for

- **Not a gate for every rule.** Some obligations are genuinely judgement, and a
  gate that encodes a judgement badly is worse than the prose. If you cannot
  state the failure precisely, write the rule and say it is unguarded.
- **Not blocking on tooling.** A point fix that ships today plus an issue for
  the gate beats a gate that never lands. Say which you did.
- **Not deleting a rule because it has no gate.** An unguarded rule is weaker,
  not void.
