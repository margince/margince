<!-- prose:plain -->
# One source of truth

**Every topic has one place that decides it**, and module boundaries decide where that place may be.
The two parts are one rule. There is no second implementation, and no copy put in a second tier to
skip an import of the owner.

"Source of truth" here means the code that *decides*, not the column that stores. Which row holds the
true value is a separate question. This principle is about the one function, fixed value, statement or
seam that every caller of a topic passes through.

The rulebook states the obligation in *Reuse before you build*. What follows is how to apply it. It
covers how to find out whether a topic already has an owner, and what to do when it has two. It also
covers how to leave a gate behind so the second one cannot come back. Read it before you add a
capability, and run its scan when you audit a part of the system.

## What counts as a topic

A topic is what a reader can name in a short line. Some examples: "how a contact's name is put in one
form so we can find duplicates", "how the pipeline value is worked out". Also "what a retention
`anonymize` removes", "how we build a system prompt for mail", "how we read a URL the tenant gave". If
you cannot name it in a short line, you are most likely looking at two topics.

Two implementations of one topic are two answers to one question. Nothing makes you ask the two
together, so they drift until they disagree where a user sees it. The third author meets the cost: they
run grep, find one of the two, and do not know the other exists. A choke point is one named function,
one SQL text, one fixed value, or one seam file that every caller of that topic goes through. It is not
an added level of code, a type that others build on, or an abstraction.

## Kinds of topic and where their choke point is

The tree already places these. Use the table to find the one that exists before you write code.

| Kind of topic | The choke point | Where |
|---|---|---|
| Writing a domain row | the store function of the module, and `storekit.Audit`/`Emit` inside one transaction | `modules/<name>/`, `platform/database/storekit` |
| Reading all of one record | `contact360.Service.Assemble` / `company360.Service.Assemble`, reached through the `Assembler` interface each caller declares | `compose/contact360`, `compose/company360` |
| An MCP tool that answers a question a page already answers | a `compose/*seam*.go` file | `internal/compose/` |
| A system prompt for a model that writes mail | `draftrules.Shared` | `compose/draftrules` |
| Reading a URL a tenant gave | `platform/webread` (with SSRF checks from `platform/netguard`) | `platform/webread` |
| The words of a filter | `storekit.FilterSet` / `predicate.go` | `platform/database/storekit` |
| Paging through a list by key | `storekit.EncodeCursor`/`DecodeCursor`/`Page` | `platform/database/storekit` |
| Something a user can click or type into in the app | the design system | `frontend/src/design-system/` |
| An error value a client can see | `shared/apperrors` | `internal/shared/apperrors` |
| An outbound event | `event_outbox` through `storekit.Emit`, shipped by `platform/events.Relay` | never an XADD of its own |

A topic that is not in the table may still have a choke point; run the scan.

## The boundary half

Where the owner may be is what a gate needs to hold the rule. The DAG is
`shared → platform → modules → compose → cmd`. One result of it is the source of most of the
duplicates in this tree:

**Modules import no sibling and not `compose`.**

Say capability A in one module needs what capability B in a second module already decides. Then the
right path is always more work than a copy:

| A needs B, where B is in | The approved move |
|---|---|
| `shared/` or `platform/` | **Import it.** No seam needed; that is what these tiers are for. |
| a sibling module | **A seam.** `compose` injects the edge as one named function. |
| `compose/` (a package under it) | **A seam.** Same shape; `compose` connects its own package to the interface the module asks for. |
| the same module | **Call it.** If it is private to the package and you need it at some other place in the package, that is not a boundary problem. |

A copy is never on this list. A seam looks like too much work next to a copy of 15 lines, and the seam
is still the answer. Ask for the seam in review.

Two rules follow from this, and both have failed here before:

- **A module writes only the tables it owns**, declared in its `doc.go` and checked by
  `backend/gates/tableownership_test.go`. A write into a sibling's table goes over the module boundary.
- **A package-keyed gate misses an in-package duplicate.** The table owner test keys its
  waivers `<package>:<table>`, so it never sees a second writer in the same module. Boundary gates and
  duplicate gates find other things; you need both.

## The scan, in this order

The steps that cost least come first, and each step finds a kind of problem the step before it cannot.

### 1. Grep the topic's key words in the whole tree, never only your folder

```shell
git grep -n "<noun>" origin/main -- backend frontend/src extensions
```

Two rules decide whether this step works at all:

- **Grep `origin/main`, not your local copy.** A shared or old worktree makes a missing file look like
  proof against a claim.
- **Grep the whole tree.** The duplicate is most often not in the package you are editing, which is why
  no one finds it.

Grep for the word in three forms, because the duplicate most often uses other words than yours. Look
for the domain word (`brief`, `weighted`, `anonymize`), the way it works (`normalize`, `Casefold`,
`ToLower`), and the thing it makes (`_profile_field`, `system prompt`, `Badge`).

**Then check the other language.** Most often one side works out the value and the other shows it,
which is the right shape. Where both decide the same rule, the two parts are one topic and not two
findings. Errors on the two sides of a wire can *hide* each other. Each half looks right in place, and
to fix one half breaks the product. The case of the money unit is in
[find the other side](derive-the-obligation.md#find-the-other-side-before-you-fix-this-one).

### 2. Count the writers of each table

For a topic that ends in a row, the census misses nothing:

```shell
git grep -nE "(INSERT INTO|UPDATE|DELETE FROM) <table>" origin/main \
  -- backend/internal/modules backend/internal/compose | grep -v _test
```

Include `DELETE`. Say one package writes the rows of a table and a second package removes them. That
table has two owners, and a census of only `INSERT` and `UPDATE` reports it as clean.

More than one writer outside tests is a finding to *decide on*, and not always a bug: each writer may
do a separate action. Answer in the code whether a new rule about this table has one place to go, or
N. Record the answer next to the writers.

### 3. Count the importers of each capability package

A capability with one importer outside tests serves one surface. Some of the time that is right, as
for a helper used inside one package. Some of the time it is the finding: the other surface built its
own.

```shell
git grep -l "internal/compose/<pkg>\"" origin/main -- 'backend/*.go' | grep -v _test
```

### 4. Read the claims that something is the only one, and do not trust them

Grep the tree for text that claims a choke point:

```shell
git grep -n "the one spelling\|the only writer\|cannot drift\|the same .* the .* performs\|there is no other" origin/main -- backend frontend
```

Of 10 such claims counted in this tree, 9 proved false. Some claims checked by hand, not counted,
proved true, but no test checked them, so nothing could see when they stopped being true. The claim is
where the duplicate hides, because the next author reads it and stops looking. For each match, run
step 1 or 2 against the thing it claims. A claim no test holds is removed or gated.

### 5. Diff the two parts you already know about

A topic can have two good implementations. Some examples: a rule in SQL and its copy in Go, or an Art. 17
erase request and a retention `anonymize`. Or a site builder and a tool builder. **Diff what each one
touches** instead of reading them to see if they look the same. Counts of each name cost the least:

```shell
grep -c "field_provenance" erasure.go retentionactions.go
```

A 0 on one side of the two is the whole finding. Reading to see if they look the same finds nothing;
counting does.

### 6. Ask what a change to the rule must edit

*If a rule about this topic changed the next day, how many files must the change touch*? If the
answer is more than one, one of two things is true. One: the topic has no choke point yet, even if the
comments say it has one. Two: it has a **declared split**, the good case with many sites.

The test is
whether each site says, in the code, what makes it other than the rest. A split that no one declared
is the finding; a declared one is a decision someone already wrote down. This step showed the 5
writers of `contact_profile_field` and the 4 of `relationship`.

## The rules, and what each one cost

The rulebook states these in *Reuse before you build*. These are the cases behind them.

**Search the whole tree.** Someone wrote the agent tool `prep_for_meeting` next to a working
`compose/meetingbrief/`. Only the HTTP handler set imported it, and a grep for one word finds it. The two answered one question with other rules about which sources to trust, until
someone wrote a seam.

**Tools and web pages share one engine.** The seams state the rule in their own words:
`compose/briefseam.go` reads `one queue rather than two readings of it`; `compose/importseam.go`
reads `delegates rather than reimplementing`. If no seam exists for what you need, write the seam.

**Never hand-type a SQL placeholder.** Derive `$N` from the list of arguments (`args = append(args, v)`
then `fmt.Sprintf("%s = $%d", col, len(args))`, as `deals/offer_lines.go` does) or use
`storekit.InsertFragments`. Nothing in this repository checks that the column, placeholder and argument
counts of a statement agree. A statement with typed numbers shipped in `contacts/researchclaim.go`.
The accept path then failed for two days, because no test called it.

The `%s` there is the column, and it has its own rule. It is a literal fixed at compile time, or a
catalog name quoted with `pgx.Identifier.Sanitize` (`storekit/customcolumns.go`). It is never a string
from a request body. Values are always `$N`. Only identifiers are formatted in, and an identifier from
a caller is an injection.

**A gate that types in its subject** copies it; see
[derive the obligation](derive-the-obligation.md#writing-a-gate-that-holds). Derive the gate's corpus
from the owner it checks (`freemail.Domains()`, the contract, the tree), or say in the test why you
cannot.

## What to do with a finding

Choose one of these results and name it, in this order. If no one names the result, a duplicate gets
through review.

1. **Keep one.** One implementation is kept; the others call it and are removed. Best when both are in
   the same import tier.
2. **Seam.** The two are in tiers that cannot import each other (a module may not import a sibling
   or `compose`). So `compose` injects the edge: one function that goes over, named for its question.
3. **Declare the split.** The two are separate capabilities that read the same. Say so in the code,
   at both sites, and name what makes them not the same. A reason in the pull request does not count,
   because the next reader is in the file.
4. **Gate it.** With any result, leave a fitness function, so the answer holds without a human to
   watch it. See below.

## Leaving a gate behind

A rule with no gate does not hold here. The frontend rulebook asks authors to open the catalog first,
every time, and the tree still has a third version of a card. The shape this repository uses for a
reuse gate follows `agenttoolparity_test.go` and `tableownership_test.go`.

Derive what you expect from the tree, and keep the way out at the subject. Approve the one case, and
write the test for the bug first. Then add the shape the gate cannot see. The way to do this is in
[derive the obligation](derive-the-obligation.md#writing-a-gate-that-holds).

### A gate is code, so it gets duplicates like code

**Moving half a rule splits it.** Some of this tree's design gates scan TypeScript *and* CSS. Say you
move the TypeScript part to an AST test and leave the CSS part in shell. That leaves one rule with two
implementations, and nothing makes you edit them together. Both parts move, or no part does. The rule
and its set of cases are kept in one place, and only the parser may change.

**Share the walk at the second gate.** The first gate has nothing to check against; by the
third there are already three answers. Two gates that scan source once each owned a copy of the
extension walk. They disagreed about which files the build tool finds, so one gate checked a unit and
the other missed it. A gate with a smaller walk reads a smaller tree and still reports PASS, with no
failing check to see. The walk is now in `frontend/scripts/lib/source-tree.ts`, with its test next to
it.

Separate calls into the TypeScript tool also disagreed about the language form, and read a `.ts` file
as TSX. So `parseSource` and `sourceFileAt` in the same file are the one parser, and its test fails a
second call site by name.

## What this does not ask for

- **Not removing duplicates only because they are copies.** Two things that look the same and answer
  separate questions are still two things, with the reason in the code. Merging them is how a gate
  stops seeing the case it exists to find.
- **Not an abstraction before a second caller.** The choke point exists because two callers need one
  answer today, not because a third may need it later. An abstraction with one caller is a
  craftsmanship finding (`T3 premature-abstraction`).
- **Not writing it all a second time.** The right result that costs least is most often a change to
  one function or one seam file.
