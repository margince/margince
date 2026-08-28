# Gate patterns — how to pick one

A **gate** is a normal Go test that reads the source tree and fails your PR when
a rule is broken. `backend/gates/` holds a lot of them (`ls backend/gates/*_test.go | wc -l`
for today's count). They are not that many unrelated tests. They come in eight
shapes, and each shape fails in its own predictable way.

Read this page **before writing a gate**, to pick the right shape.

- The full list of gates, grouped by shape, is generated:
  [gate-inventory.md](gate-inventory.md).
- The rules for making a gate hold are in
  [derive-the-obligation.md](../principles/derive-the-obligation.md).
- What will fail your PR today is in
  [backend-onboarding.md](../explanation/backend-onboarding.md).

---

## Step 1: how strong can your gate be?

Three levels. Aim for the highest one your subject allows, then say in the test
where you had to stop.

| Level | How it reads the tree | Can it miss something? |
|---|---|---|
| **H3 — total** | Compares two *lists*. Every item on one side is matched against every item on the other. | **No.** A new item shows up in one list and the diff names it. |
| **H2 — structural** | Parses Go into an AST and walks it. | **Yes** — at any call it cannot resolve: through an interface, a stored field, or a closure. |
| **H1 — textual** | Runs a regex over source or SQL text. | **Yes** — at any spelling the regex does not cover, such as a query built with `+`. |

H1 is not cheating. Go sends SQL as plain strings, so most SQL gates *have* to be
H1. The difference between a good H1 gate and a useless one is that a good one
**says which spellings it cannot see** and **counts what it found**, so a broken
regex fails instead of quietly finding nothing.

---

## Step 2: pick the shape

Find the sentence that matches your rule.

| Your rule sounds like… | Shape |
|---|---|
| "These two places store the same fact and must agree." | [A. Parity](#a-parity) |
| "Everything in the list must exist, and everything that exists must be in the list." | [B. Census](#b-census) |
| "Every function that does X must also call Y." | [C. Reachability](#c-reachability) |
| "Every query/declaration like this must include Z." | [D. Shape](#d-shape) |
| "This must not appear anywhere (except here)." | [E. Prohibition](#e-prohibition) |
| "This comment says it's the only one — prove it." | [F. Claim check](#f-claim-check) |
| "This number must not grow." | [G. Budget](#g-budget) |
| "Does my new gate actually reject anything?" | [H. Falsification](#h-falsification) |

---

### A. Parity

**Checks:** the same fact is written in two places, and they match.

**How:** build list A from the owner and list B from the copy, separately. Then
assert **both** differences are empty — `A minus B` and `B minus A`. Never just
"A contains B". Write down in the test which side is the owner, so the next
person knows which one to fix.

**Hardness:** H3 if both sides are lists. H1 if one side is text.

**Use when** a value must exist twice because the two readers can't share code —
across a language boundary (Go ↔ TypeScript), across a process boundary, or
across contract ↔ schema ↔ Go.

**Examples:** `frontendminorunits_test.go` (money scale in the browser vs the
server) · `enumsync_test.go` (Go enum vs the schema's `CHECK`) ·
`goversionpins_test.go` · `sendattachmentcap_test.go` (one limit written in five
places).

**⚠ How it silently passes:** both sides read from the *same* source, so the test
compares a value to itself. This happens most often when side B is read out of a
generated file that was generated from side A.

**Fix:** trace where each side actually comes from, and assert the list is not
empty.

> Why this matters: a money scale that is wrong on both sides cancels out. The
> screen agrees with itself, and the customer sees 100× the real price on the
> offer they sign.

---

### B. Census

**Checks:** a registry is complete, in both directions.

**How:** build one list by scanning the tree ("what exists") and another from the
declaration ("what we claim exists"). Diff both ways.

- Declared but missing → a stale entry.
- Exists but not declared → the bug you wrote the gate for.

**Hardness:** H2 or H3. The scanning half is the weak one.

**Use when** there is a catalog, contract, registry, or `doc.go` list that should
name everything of a kind: jobs, AI tasks, event types, PII tables, tools.

**Examples:** `jobcensus_test.go` (`jobs.yaml` ↔ what compose actually wires) ·
`piicoverage_test.go` (every PII table is reached by deletion) ·
`registrarparity_test.go` · `contractproducers_test.go` (a field the API promises
but nothing writes).

**⚠ How it silently passes:** the scan reads a **smaller tree than you think** —
a glob stopped matching after a rename, or a skip-list excludes the one bad file.
It finds nothing and reports PASS. Nothing fails, so nobody notices.

**Fix: count what you found, and fail if the count is too low.**

```go
// A census that judged nothing certifies nothing. The floor sits BELOW the
// real count, so it catches a broken scan rather than a changing tree.
if named < 4 || assembled < 1 {
    t.Fatalf("found %d named and %d assembled writer(s) — one of the two ways "+
        "of finding a statement has stopped working", named, assembled)
}
```

Use **two counts, not one**, when the scan has two halves. A single total hides
which half broke. If your gate walks a subdirectory, use `gatekit.Scope`: it also
checks the code *outside* your walk root, which proves the root is the right one.

---

### C. Reachability

**Checks:** every function that does X also calls Y somewhere on its call path.

**How:** build a call graph of the package. Key it by **receiver type + function
name**, not by name alone — `apply` is a method on one service and also a common
name everywhere else. Find the functions that do X, then check each one reaches Y.

**The quantifier is the whole gate.** Two ways to write it, and only one works:

| Version | Result |
|---|---|
| "*some* function above this one can also reach Y" | **Useless.** If Y is called from seven places, this is true of almost everything. |
| "this function calls Y itself, **or** it has callers and **all** of them are guarded" | Correct. A function with no callers is an entry point — if it hasn't called Y by then, nothing will. |

**Hardness:** H2. A call through an interface, a stored field, or a closure is
invisible to the walk. Say that in the test. Never claim those paths carry nothing.

**Use when** the rule is about *pairing or ordering*, not shape: an audit row owes
an outbox event; a rename owes a duplicate check; a write owes a permission probe.

**Examples:** `writeshape_test.go` (audit row ⇒ outbox event on the same path) ·
`writeauthorityreach_test.go` · `rbacgate_test.go` · `personscrub_test.go`
(deleting and anonymising a person clear the same tables) · `dedupespine_test.go` ·
`orgrenamerecheck_test.go` (every organisation rename reaches the duplicate check —
the gate whose first version was vacuous, which is why the quantifier table above
exists).

**⚠ How it silently passes:** the weak quantifier above. It looks exactly like a
gate that works.

**Fix: mutation-test it before you merge.** Delete the required call from each
real call site, one at a time, and watch the gate fail naming the right function.
Two rules:

1. **Check each mutant compiles.** A mutant that doesn't compile proves nothing.
2. **Print the failure message yourself, once.** Swapped arguments — printing the
   writer's name where the target belongs — is a live bug in a message nobody
   ever sees.

---

### D. Shape

**Checks:** every statement or declaration of a kind includes a required part.

**How:** collect the population (SQL statements, exported methods, struct tags),
then check each one on its own. Unlike C, no call graph is needed.

**Hardness:** H1 for SQL text, H2 for AST declarations.

**Use when** you can decide by looking at one statement in isolation.

**Examples:** `updateguard_test.go` (every single-row `UPDATE` has *some*
concurrency guard) · `tableownership_test.go` (a module only writes its own
tables) · `errmatch_test.go` (classify DB errors by SQLSTATE, never by message
text) · `positionalrowscan_test.go`.

**⚠ How it silently passes:** a spelling the regex cannot see. Real example from
this repo — `organization_profile_field_write.go` builds its query like this:

```go
`UPDATE organization SET ` + column + ` = $2`   // column is "display_name" at runtime
```

A regex looking for `display_name` finds nothing, so the gate does not see this
file as a writer at all. The rule was unenforced for a shape the codebase
already had.

**Fix — three things:**

1. **Also match the assembled form.** A query that ends at `SET` gets judged like
   a named one: the gate can't know which column arrives, so it asks the same
   question.
2. **Strip SQL comments and quoted strings first.** Without it,
   `SET description = $1 -- display_name =` counts as a rename, and a `;` inside a
   comment ends a non-greedy match early and hides a real one on the next line.
3. **Count the two forms separately**, so a floor can tell you which one broke.

---

### E. Prohibition

**Checks:** a pattern does not appear anywhere it shouldn't.

**How:** scan the **whole** corpus — not just the folder you expect the problem
in — and assert zero matches outside approved sites. Here the corpus definition
*is* the gate.

**Hardness:** H1–H2. The weak point is the corpus, not the regex.

**Use when** the correct answer is "nowhere" or "only here": no direct River
registration, no `SELECT id FROM workspace` inside a module, no flag default read
from the environment, no credential in a log field.

**Examples:** `jobregistrationban_test.go` · `flagdefault_test.go` ·
`logsecrets_test.go` · `formulafieldscope_test.go` · `publicreferences_test.go`.

**⚠ How it silently passes:** the regex is anchored on the *obvious* spelling.
Real example — `rulebookdirection_test.go` bans links from `docs/` up to a
rulebook. Its first version matched `(?:\.\./)+`, which is the obvious way to say
"goes up a directory". It could not see `](/AGENTS.md)` or `](frontend/AGENTS.md)`.

**Fix:** derive the banned set from **the thing that defines it** — River's actual
API, the real filesystem — instead of from a list of spellings you remember. Then
ask what version of the bug your regex still can't see, and add that case.

---

### F. Claim check

**Checks:** a comment saying "this is the only X" is actually true.

**How:** a detector finds the claim's *phrasing* in a doc comment, works out which
declaration it belongs to, and holds the tree to it. `uniquenessclaims.txt` is the
register of recognised claims.

**Hardness:** H1. It can only hold a phrasing it recognises.

**Use when** you are about to write "the only", "the one spelling of", or "spelled
once" in a comment. That sentence stops the next person searching: they grep, find
your claim, and stop. A false one is worse than no comment at all.

**Examples:** `uniquenessclaims_test.go` and its detector ·
`consumermailonelist_test.go` · `employmentcurrency_test.go`.

**⚠ How it silently passes:** a phrasing the detector doesn't know. This is not
theoretical. `companyform.go` said *"this function is the only writer of that
column a human drives"*. It was false, and the register never saw it, because that
wording wasn't a shape the detector recognises. The false comment that motivated a
whole new gate walked straight past the gate built to catch false comments.

**Fix:** before writing the claim, check the detector recognises your wording — or
use wording it does. Widening the detector is a separate change, because it will
turn up claims that were already there.

---

### G. Budget

**Checks:** a cost stays under a stated limit.

**How:** compute the cost from the tree or config, compare to a constant, and put
the reason for the number next to it.

**Hardness:** usually H3 — the number is computable.

**Use when** the cost is paid on every session, every CI run, or every connection.

**Examples:** `rulebooklength_test.go` (every session reads the rulebook in full) ·
`laneconnbudget_test.go` (connections = concurrent packages × per-package limit) ·
`workflowtimeouts_test.go` (a job with no `timeout-minutes` inherits GitHub's
6-hour default).

**⚠ How it silently passes:** the **ratchet**. A limit set to "whatever it is
today", raised every time it fails, is not a budget — it's a log of the growth it
was supposed to stop.

**Fix:** write down what the number buys. Make raising it a decision with a reason,
not a one-character edit.

---

### H. Falsification

**Checks:** your new gate actually rejects something.

**How:** a companion test with **fake** inputs — every shape the real tree uses,
proven accepted, plus the shape the gate exists to reject, proven rejected.

**Use when** the gate is non-trivial: any reachability gate, any whole-tree sweep,
any detector. "The tree is currently clean" proves nothing about your gate.

**Examples:** `jobkindgate_test.go` and `jobfleetwideshapes_test.go` (each kept
next to the gate it falsifies) · `extensionsqlscopecases_test.go` ·
`uniquenessclaimsdetector_test.go`.

One level up, `gatecensus_test.go` checks the gate machinery itself: a gate's
exceptions must meet the same standard the gate applies to its subjects.

---

## Don't rebuild the plumbing

| Helper | What you get |
|---|---|
| `gatekit.Waive` | A waiver that must state what it costs (≥20 chars, and restating the subject doesn't count). `AssertAllMatched` fails a waiver that no longer matches anything — otherwise it quietly exempts whatever inherits that name later. |
| `gatekit.Scope` | Proves your walk root is the right one, by also checking the code outside it. |
| `gatekit.LiteralText` | One way to decode a Go string literal into the SQL it sends — quoted or escaped. |
| `gatekit.StringExpr` | The shared "what string does this Go expression hold" reader, with the question a parameter: `FoldStrict` for "is this DEFINITELY this string" (an unresolvable part means not a string), `FoldTotal` for "what can I SEE of this string" (an unresolvable part becomes `ComputedFragment` and the fold carries on). Both are in use; writing your own decides which shapes your census is blind to. |
| `gatekit.SQLStatementsIn` | The shared "what SQL does this file send" reader: one entry per statement, escapes decoded, `+` chains flattened. `SQLStatementsOf` takes a parsed subtree, `SQLTextOf` joins the statements with newlines for a line-scanning reader. Reading `ast.BasicLit.Value` yourself gets SOURCE text, so a statement written in double quotes arrives with `\n` as two characters and your census reports a clean tree. |
| `sqlhelperwalk_test.go` | The shared walk over a package's SQL helpers, so two censuses read the same set. |
| `callgraph_test.go` | The shared call graph, keyed by receiver type so a method is told from a plain function of the same name. Returns **statements**, so each census decides what a statement means. |

Copying one of these for a second caller is how two gates drift apart. The copy
walks a smaller tree and reports PASS.

---

## Before you merge a gate

1. Shape chosen from the rule, not from the file you were already in.
2. Subject list **derived from the tree**, never hardcoded. A hardcoded list goes short.
3. A **count floor** — two floors if the scan has two halves.
4. **Mutation-tested**: mutants applied, compiled, and watched failing with the right name.
5. Failure message **printed once by you**. Check the argument order.
6. Escape hatch **next to the subject** where possible — a `doc.go` line, a contract
   field, a `//craft:ignore <check> <reason>`. A map inside the test file is invisible
   to exactly the person who needs it. If you can't, say in the test why.
7. Waive the **instance, not the category**. A waiver keyed by package or rule name
   lets the next offender in free.
8. An **empty waiver map is a result** — say so in a comment, so adding an entry is a
   visible decision rather than a silent edit to the regex.
9. **Say what the gate can't see**: unresolvable calls, assembled strings, unknown
   phrasings.
10. Tag the file (see [gate-inventory.md](gate-inventory.md)) so it appears in the list.

---

## When a chokepoint beats a gate

A gate holds a rule people would otherwise have to remember. A **chokepoint** —
one function that can't be called wrong — removes the remembering. Prefer it when
the callers look alike and the whole obligation fits in one call.

They are not alternatives. The audit+event rule has both: `storekit.Audit` /
`Emit` is the chokepoint, and `writeshape_test.go` is still the gate, because
nothing stops the next person writing the pair by hand next to it. **The
chokepoint makes the gate simple; the gate keeps the chokepoint the only door.**

Two things a chokepoint usually can't absorb, both live in this codebase:

- **Lock ordering.** `lockOrgNameWrites` must be taken *before* locking the row it
  protects. A helper that takes it on entry takes it *after* the caller already
  locked the row — baking in the deadlock it was meant to prevent.
- **Once-per-transaction timing.** A check that must run once after a whole
  multi-field update can't live in a per-column helper without firing N times,
  each under a workspace-wide advisory lock held until commit.

Where those bite, wrap the **obligation**, not the SQL: the write records that it
happened, and one step near commit discharges it. The gate then shrinks from
"every writer reaches the check through all-callers-guarded reachability" to
"nothing outside this file touches these columns" — a stronger claim with far less
machinery.
