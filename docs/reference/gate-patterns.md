<!-- prose:plain -->
# Gate patterns: how to pick one

A **gate** is a normal Go test that reads the source tree and fails your PR when someone breaks a
rule. `backend/gates/` holds many of them (`ls backend/gates/*_test.go | wc -l` gives the count for
today). They come in 8 shapes, and each shape fails in its own way, which you can see coming.

Read this page before you write a gate, to pick the right shape.

- The full list of gates, sorted by shape, is generated:
  [gate-inventory.md](gate-inventory.md).
- The rules that make a gate hold are in
  [derive-the-obligation.md](../principles/derive-the-obligation.md).
- What will fail your PR today is in
  [backend-onboarding.md](../explanation/backend-onboarding.md).

---

## Step 1: how strong can your gate be?

There are three levels. Go for the highest one your subject lets you reach, then say in the test
where you had to stop.

| Level | How it reads the tree | Can it miss something? |
|---|---|---|
| **H3: total** | Holds two *lists* side by side. It matches every item on one side against every item on the other. | No. A new item shows up in one list, and the diff names it. |
| **H2: structure** | Reads Go into an AST and walks it. | Yes, at each call it cannot follow: through an interface, a stored field, or a closure. |
| **H1: text** | Runs a regex over source or SQL text. | Yes, at each way of writing that the regex does not cover, such as a query built with `+`. |

H1 is a good level to use. Go sends SQL as plain strings, so most SQL gates *have* to be H1. A good
H1 gate says which forms it cannot see, and counts what it found. That way a broken regex fails,
and does not find nothing in silence.

---

## Step 2: pick the shape

Find the sentence that matches your rule.

| Your rule reads like… | Shape |
|---|---|
| "These two places store the same fact and must agree." | [A. Parity](#a-parity) |
| "Everything in the list must exist, and everything that exists must be in the list." | [B. Census](#b-census) |
| "Every function that does X must also call Y." | [C. Reachability](#c-reachability) |
| "Every query/declaration like this must include Z." | [D. Shape](#d-shape) |
| "This must not show up in any place (but here)." | [E. Prohibition](#e-prohibition) |
| "This comment says it is the only one; prove it." | [F. Claim check](#f-claim-check) |
| "This number must not grow." | [G. Budget](#g-budget) |
| "Does the new gate really refuse anything?" | [H. Falsification](#h-falsification) |

---

### A. Parity

**Checks:** the same fact is written in two places, and the two match.

**How:** build list A from the owner and list B from the copy, each on its own. Then check that
both diffs are empty: `A minus B` and `B minus A`. Never just "A has B in it". Write down in the
test which side is the owner, so the next author knows which one to fix.

**How strong:** H3 if both sides are lists. H1 if one side is text.

**Use when** a value must be in two places because the two readers cannot share code. That is
across Go and TypeScript, across two programs, or across contract ↔ schema ↔ Go.

**Examples:**

- `frontendminorunits_test.go` (money scale in the web app against the server).
- `enumsync_test.go` (Go enum against the `CHECK` in the schema).
- `goversionpins_test.go`.
- `sendattachmentcap_test.go` (one limit written in 5 places).

**⚠ How it passes in silence:** both sides read from the *same* source, so the test checks a value
against itself. This happens most often when side B comes from a generated file that was generated
from side A.

**Fix:** trace where each side really comes from, and check that the list is not empty.

> Why this matters: a money scale that is wrong on both sides cancels out. The screen agrees with
> itself, and the customer sees 100× the real price on the offer they sign.

---

### B. Census

**Checks:** a register is full, in both ways.

**How:** build one list by a scan of the tree ("what exists") and another from the declaration
("what we claim exists"). Diff both ways.

- Declared but missing → an old entry.
- Exists but not declared → the bug you wrote the gate for.

**How strong:** H2 or H3. The scan part is the weak one.

**Use when** a catalog, contract, register, or `doc.go` list should name everything of a kind:
jobs, AI tasks, event types, PII tables, tools.

**Examples:** `jobcensus_test.go` (`jobs.yaml` ↔ what `compose` really wires) ·
`piicoverage_test.go` (a delete reaches every PII table) · `registrarparity_test.go` ·
`contractproducers_test.go` (a field the API promises but nothing writes).

**⚠ How it passes in silence:** the scan reads a smaller tree than you think. A glob stopped
matching after a rename, or a skip list leaves out the one bad file. It finds nothing and reports
PASS. Nothing fails, so nobody finds out.

**Fix:** count what you found, and fail if the count is too low.

```go
// A census that judged nothing certifies nothing. The floor sits BELOW the
// real count, so it catches a broken scan rather than a changing tree.
if named < 4 || assembled < 1 {
    t.Fatalf("found %d named and %d assembled writer(s) — one of the two ways "+
        "of finding a statement has stopped working", named, assembled)
}
```

Use two counts when the scan has two parts; one total hides which part stopped working. If your
gate walks a folder under the tree, use `gatekit.Scope`. It also checks the code *outside* your
walk root, which proves the root is the right one.

---

### C. Reachability

**Checks:** every function that does X also calls Y on its call path.

**How:** build a call graph of the package. Key it by receiver type + function name, because
`apply` is a method on one service and also a common name in other places. Find the functions that
do X, then check that each one reaches Y.

**Whether you write *some* or *all* decides whether the gate works.** There are two ways to write
it, and only one works:

| Version | Result |
|---|---|
| "*some* function above this one can also reach Y" | **Of no use.** If 7 places call Y, this is true of most code. |
| "this function calls Y itself, **or** it has callers and **all** of them are guarded" | Right. A function with no callers is an entry point: if it has not called Y by then, nothing will. |

**How strong:** H2. The walk cannot see a call through an interface, a stored field, or a closure.
Say that in the test. Never claim those paths carry nothing.

**Use when** the rule is about pairs or order, not shape:

- an audit row owes an outbox event;
- a rename owes a check for copies;
- a write owes a permission check.

**Examples:**

- `writeshape_test.go` (audit row ⇒ outbox event on the same path).
- `writeauthorityreach_test.go` · `rbacgate_test.go` · `dedupespine_test.go`.
- `contactscrub_test.go` (to delete a contact and to remove its personal data empty the same tables).
- `companyrenamerecheck_test.go` (every company rename reaches the check for copies). Its first
  version used the weak *some* form and passed everything.

**⚠ How it passes in silence:** the weak *some* form above. It looks the same as a gate that works.

**Fix: mutation-test it before you merge.** Delete the needed call from each real call site, one at
a time, and watch the gate fail and name the right function. Two rules:

1. **Check that each mutation compiles.** A mutation that does not compile proves nothing.
2. **Print the failure message yourself, once.** Arguments in the wrong order are a live bug in a
   message nobody reads. One name shows where another belongs.

---

### D. Shape

**Checks:** every statement or declaration of a kind has a part it needs.

**How:** take the full set (SQL statements, public methods, struct tags), then check each one on
its own. You need no call graph, as you do in C.

**How strong:** H1 for SQL text, H2 for AST declarations.

**Use when** you can decide by looking at one statement by itself.

**Examples:**

- `updateguard_test.go` (every `UPDATE` of one row has *some* guard against two writes at once).
- `tableownership_test.go` (a module writes only its own tables).
- `errmatch_test.go` (sort DB errors by `SQLSTATE`, never by message text).
- `positionalrowscan_test.go`.

**⚠ How it passes in silence:** a form the regex cannot see. In this repository,
`company_profile_field_write.go` builds its query like this:

```go
`UPDATE company SET ` + column + ` = $2`   // column is "display_name" at runtime
```

A regex that looks for `display_name` finds nothing, so the gate does not see this file as a writer
at all. No gate kept the rule for a shape the code already had.

**Fix:**

1. **Also match the built form.** A query that ends at `SET` gets judged like a named one. The
   gate cannot know which column comes next, so it asks the same question.
2. **Remove SQL comments and strings in quotes first.** Without that,
   `SET description = $1 -- display_name =` counts as a rename. And a `;` inside a comment ends a
   short match early and hides a real one.
3. **Count the two forms on their own**, so a floor can tell you which one stopped working.

---

### E. Prohibition

**Checks:** a pattern does not show up in any place it should not.

**How:** scan the whole body of text, not only the folder where you expect the problem. Check that
there are no matches outside the approved sites. Here what you scan *is* the gate.

**How strong:** H1 to H2. The weak point is what you scan, not the regex.

**Use when** the right answer is "in no place" or "only here":

- no River job register by hand;
- no `SELECT id FROM workspace` inside a module;
- no flag default read from the environment;
- no credential in a log field.

**Examples:** `jobregistrationban_test.go` · `flagdefault_test.go` · `logsecrets_test.go` ·
`formulafieldscope_test.go` · `publicreferences_test.go`.

**⚠ How it passes in silence:** the regex looks for the *plain* form only. For example,
`rulebookdirection_test.go` bans links from `docs/` up to a rulebook. Its first version matched
`(?:\.\./)+`, the plain way to say "goes up a folder". It could not see `](/AGENTS.md)` or
`](frontend/AGENTS.md)`.

**Fix:** derive the banned set from the thing that defines it, such as the real River API or the
real file system. Do not derive it from a list of forms you remember. Then ask what version of the
bug your regex still cannot see, and add that case.

---

### F. Claim check

**Checks:** a comment that says "this is the only X" is really true.

**How:** a detector finds the *words* of the claim in a doc comment. It works out which declaration
the claim belongs to, and holds the tree to it. `uniquenessclaims.txt` is the register of the
claims it knows.

**How strong:** H1. It can only hold words it knows.

**Use when** you are about to write `the only`, `the one spelling of`, or `spelled once` in a
comment. That sentence stops the next author from searching: they grep, find your claim, and stop.
A false claim is worse than no comment at all.

**Examples:** `uniquenessclaims_test.go` and its detector · `consumermailonelist_test.go` ·
`employmentcurrency_test.go`.

**⚠ How it passes in silence:** words the detector does not know. `companyform.go` said that this
function was the only writer of that column a human edits. It was false, and the register never
found it, because the detector did not know that form of words.

**Fix:** before you write the claim, check that the detector knows your words, or use words it does
know. Make the detector know new forms in a separate change, because it will turn up claims that were
already there.

---

### G. Budget

**Checks:** a cost stays under a limit you set.

**How:** work out the cost from the tree or the config, and check it against a constant. Put the
reason for the number next to it.

**How strong:** most often H3, because you can work out the number.

**Use when** the cost comes with every session, every CI run, or every connection.

**Examples:**

- `rulebooklength_test.go` (every session reads the whole rulebook).
- `laneconnbudget_test.go` (connections = packages at once × limit per package).
- `workflowtimeouts_test.go` (a job with no `timeout-minutes` gets the 6-hour GitHub default).

**⚠ How it passes in silence:** the **ratchet**. A limit set to "what it is today", and raised each
time it fails, only records more and more of what it should stop.

**Fix:** write down what the number buys. Make it a decision with a written reason to raise it; an
edit of one character should not be enough.

---

### H. Falsification

**Checks:** your new gate really refuses something.

**How:** a second test with input made for it. The input has every shape the real tree uses, which
the gate must accept. It also has the shape the gate is there to refuse, which it must refuse.

**Use when** the gate is not small: any gate of the Reachability shape, any scan of the whole tree,
any detector. "The tree is clean today" proves nothing about your gate.

**Examples:** `jobkindgate_test.go` and `jobfleetwideshapes_test.go` (each kept next to the gate it
tests) · `extensionsqlscopecases_test.go` · `uniquenessclaimsdetector_test.go`.

One level up, `gatecensus_test.go` checks the gate code itself: the waivers of a gate must meet the
same rules the gate sets for its subjects.

---

## Do not build the shared helpers again

| Helper | What you get |
|---|---|
| `gatekit.Waive` | A waiver that must say what it costs (≥20 characters, and to name the subject again does not count). `AssertAllMatched` fails a waiver that no longer matches anything; if it did not, the waiver would cover any later code that takes that name. |
| `gatekit.Scope` | Proves your walk root is the right one, by also checking the code outside it. |
| `gatekit.LiteralText` | One way to read a Go string literal into the SQL it sends, with quotes or escapes. |
| `gatekit.StringExpr` | The shared reader for "what string does this Go code hold", with the question as a parameter. `FoldStrict` asks "is this this string, and nothing else": a part it cannot read means not a string. `FoldTotal` asks "what part of this string can be read": a part it cannot read becomes `ComputedFragment`, and it goes on. The bool then says whether any part could be read, so a hole never closes over the parts next to it. Both are in use; to write your own decides which shapes your census cannot see. |
| `gatekit.SQLStatementsIn` | The shared reader for "what SQL does this file send": one entry per statement, escapes read, `+` parts joined. `SQLStatementsOf` takes a part of the tree that is already read. `SQLTextOf` joins the statements with new lines for a reader that scans line by line. If you read `ast.BasicLit.Value` yourself, you get source text. Then a statement written in `"` quotes comes with `\n` as two characters, and your census reports a clean tree. |
| `sqlhelperwalk_test.go` | The shared walk over the SQL helpers of a package, so two gates of the Census shape read the same set. |
| `callgraph_test.go` | The shared call graph, keyed by receiver type so a method is never read as a plain function of the same name. It returns **statements**, so each census decides what a statement means. |

To copy one of these for a second caller is how two gates drift. The copy walks a smaller
tree and reports PASS.

---

## Before you merge a gate

1. Pick the shape from the rule, not from the file you were already in.
2. Derive the subject list from the tree; never type it in. A list typed in goes short.
3. A count floor; two floors if the scan has two parts.
4. Mutation-tested: mutations made, compiled, and watched as they fail with the right name.
5. You printed the failure message once yourself. Check the order of the arguments.
6. Put the way out next to the subject where you can: a `doc.go` line, a contract field, a
   `//craft:ignore <check> <reason>`. A map inside the test file does not reach the reader who
   needs it. If you cannot, say in the test why.
7. Waive the one case, not the whole kind. A waiver keyed by package or rule name lets the next
   bad case in for free.
8. An empty waiver map is a result. Say so in a comment. Then a new entry is a decision a reader can
   see, not a change to the regex.
9. Say what the gate cannot see: calls it cannot follow, built strings, words it does not know.
10. Tag the file (see [gate-inventory.md](gate-inventory.md)) so it shows up in the list.

---

## When to use a chokepoint, not a gate

A gate holds a rule a developer would have to remember. A **chokepoint** (one function that nobody
can call the wrong way) removes the need to remember. Use it when the callers look the same and
the whole rule fits in one call.

The two work together. The audit+event rule has both: `storekit.Audit` / `Emit` is the chokepoint.
`writeshape_test.go` is still the gate, because nothing stops the next author from writing the pair
by hand next to it.

Two things a chokepoint most often cannot take in, and both are live in this code:

- **Lock order.** Code must take `lockCompanyNameWrites` *before* it locks the row it guards. A
  helper that takes it on entry takes it *after* the caller already locked the row. That builds in
  the deadlock it should have stopped.
- **Once per transaction.** A check that must run once after a change to many fields cannot live
  in a helper per column without running many times. Each run holds an advisory lock over the whole
  workspace until commit.

Where those get in the way, wrap the rule, not the SQL. The write records that it happened, and one
step just before commit does the check. The gate then gets smaller. It goes from "every writer reaches the
check, through call paths where all callers are guarded" to "nothing outside this file touches
these columns". That is a stronger claim with much less code.
