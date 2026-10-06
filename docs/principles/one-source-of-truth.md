# One source of truth

**Every topic has one place that decides it**, and module boundaries decide where that place may
live. The two halves are one rule: no second implementation, and no copy carried across a tier
because importing the owner was inconvenient.

"Source of truth" here means the code that *decides*, not the column that stores. The authoritative
row is a separate question; this principle is about the one function, constant, statement or seam
every caller of a topic passes through.

The rulebook states the obligation in *Reuse before you build*. What follows is the method: how to
find out whether a topic already has an owner, what to do when it has two, and how to leave a gate
behind so the second one cannot come back. Read it before adding a capability, and run its scan when
auditing a subsystem.

## What counts as a topic

A topic is whatever a reader would name in one phrase. Examples: "how a contact's name is normalized
for dedupe", "how weighted pipeline value is computed", "what a retention anonymize scrubs", "how we
build a system prompt for correspondence", "how we fetch a URL the tenant supplied". If you cannot
name it in a phrase, you are probably looking at two topics.

Two implementations of one topic are two answers to one question. Nothing forces the two to be asked
together, so they drift until they disagree in front of a user. The cost lands on the third author,
who greps, finds one of them, and does not know the other exists. A choke point is one named
function, one SQL literal, one constant, or one seam file that every caller of that topic goes
through. It is not a layer, a base class or an abstraction.

## Topic kinds and where their choke point lives

The tree already places these. Use the table to find the existing one before writing anything.

| Topic kind | The choke point | Where |
|---|---|---|
| Writing a domain row | the module's store method, and `storekit.Audit`/`Emit` inside one tx | `modules/<name>/`, `platform/database/storekit` |
| Reading one record's world | `contact360.Service.Assemble` / `company360.Service.Assemble`, reached through the `Assembler` interface each consumer declares | `compose/contact360`, `compose/company360` |
| An MCP tool that answers a question a page already answers | a `compose/*seam*.go` file | `internal/compose/` |
| A model system prompt for correspondence | `draftrules.Shared` | `compose/draftrules` |
| Fetching a tenant-supplied URL | `platform/webread` (SSRF-guarded via `platform/netguard`) | `platform/webread` |
| A filter/predicate vocabulary | `storekit.FilterSet` / `predicate.go` | `platform/database/storekit` |
| Keyset pagination | `storekit.EncodeCursor`/`DecodeCursor`/`Page` | `platform/database/storekit` |
| An interactive control in the UI | the design system | `frontend/src/design-system/` |
| A sentinel error a client can see | `shared/apperrors` | `internal/shared/apperrors` |
| An outbound event | `event_outbox` via `storekit.Emit`, shipped by `platform/events.Relay` | never a direct XADD |

A topic missing from the table may still have a choke point; run the scan.

## The boundary half

Where the owner may live is what makes the rule enforceable. The DAG is
`shared → platform → modules → compose → cmd`, and one consequence of it produces most of this
tree's duplicates:

**Modules import neither siblings nor `compose`.**

So when capability A in one module needs what capability B in another already decides, the correct
path is always more work than copying:

| A needs B, where B lives in | The sanctioned move |
|---|---|
| `shared/` or `platform/` | **Import it.** No seam needed; that is what those tiers are for. |
| a sibling module | **A seam.** `compose` injects the edge as one named function. |
| `compose/` (a subpackage) | **A seam.** Same shape; `compose` binds its own subpackage to the module's injected interface. |
| the same module | **Call it.** If it is unexported and you need it elsewhere in the package, that is not a boundary problem. |

Copying is never on this list. A seam looks like ceremony next to a fifteen-line copy, and the seam
is still the answer. Ask for the seam in review.

Two corollaries, both violated here before:

- **A module writes only the tables it owns**, declared in its `doc.go` and held by
  `backend/gates/tableownership_test.go`. A write into a sibling's table crosses the module
  boundary.
- **A package-keyed gate misses an intra-package duplicate.** The ownership test keys its waivers
  `<package>:<table>`, so every second writer living inside one module is invisible to it. Boundary
  gates and duplication gates catch different things; you need both.

## The scan, in this order

Cheap first, and each probe finds a class the previous one cannot.

### 1. Grep the topic's nouns tree-wide, never your directory

```shell
git grep -n "<noun>" origin/main -- backend frontend/src extensions
```

Two rules decide whether this probe works at all:

- **Grep `origin/main`, not the checkout.** A shared or stale worktree makes an absent file look
  like a refuted premise.
- **Grep the whole tree.** The duplicate is almost never in the package you are editing, which is
  why it was missed.

Grep for the noun in three registers, because the duplicate rarely reuses your spelling: the domain
word ("brief", "weighted", "anonymize"), the mechanism ("normalize", "Casefold", "ToLower"), and the
artifact ("`_profile_field`", "system prompt", "Badge").

**Then check the other language.** Usually one side computes and the other renders, which is the
shape to want. Where both decide the same rule, the two halves are one topic rather than two
findings, because errors on opposite sides of a wire can *cancel*: each half looks correct in place,
and fixing one half is a regression. The money-scale case is written up in [find the other
side](derive-the-obligation.md#find-the-other-side-before-you-fix-this-one).

### 2. Count the writers of each table

For a topic that ends in a row, the census is complete:

```shell
git grep -nE "(INSERT INTO|UPDATE|DELETE FROM) <table>" origin/main \
  -- backend/internal/modules backend/internal/compose | grep -v _test
```

Include `DELETE`. A table whose rows one package writes and another package destroys has two owners,
and a census of inserts and updates alone reports it as clean.

More than one non-test writer is a finding to *rule on*, not automatically a defect: each writer may
be a distinct verb. Answer in the code whether a new rule about this table has one place to land, or
N, and record the answer next to the writers.

### 3. Count the importers of each capability package

A capability with one non-test importer serves one surface. Sometimes that is correct (an internal
helper of one package); sometimes it is the finding (the other surface built its own).

```shell
git grep -l "internal/compose/<pkg>\"" origin/main -- 'backend/*.go' | grep -v _test
```

### 4. Read the uniqueness claims and disbelieve them

Grep the tree for prose that asserts a choke point:

```shell
git grep -n "the one spelling\|the only writer\|cannot drift\|the same .* the .* performs\|there is no other" origin/main -- backend frontend
```

Nine of the ten such claims counted in this tree were false. Several of the claims spot-checked
rather than counted were true but held by no test, so nothing would notice when they stopped being
true. The claim is where the duplicate hides, because the next author reads it and stops looking.
For each hit, run probe 1 or 2 against the thing it claims. A claim no test holds is either deleted
or gated.

### 5. Diff the two halves you already know about

A topic can have two legitimate implementations: a SQL expression and its Go twin, an Art. 17
erasure and a retention anonymize, a website assembler and a tool assembler. **Diff what each one
touches** instead of reading them for similarity. Reference counts are the cheap version:

```shell
grep -c "field_provenance" erasure.go retentionactions.go
```

A zero on one side of a pair is the whole finding. Reading for similarity finds nothing; counting
does.

### 6. Ask what a sweep would have to edit

*If a rule about this topic changed tomorrow, how many files would the change have to touch*? If the
answer is more than one, either the topic has no choke point yet, whatever the comments say, or it
has a **declared divergence**, the legitimate multi-site case. The difference is whether each site
says, in the code, what makes it different. An undeclared N is the finding; a declared N is a
decision somebody already made and wrote down. This probe found `contact_profile_field`'s five
writers and `relationship`'s four.

## The rules, and what each one cost

The rulebook states these in *Reuse before you build*. These are the incidents behind them.

**Search the whole tree.** The agent tool `prep_for_meeting` was written beside a working
`compose/meetingbrief/`, whose only importer was the HTTP handler set and which a one-word grep
would have found. The two answered one question with different grounding rules until a seam was
written.

**Tools and web pages share one engine.** The seams state the rule in their own words:
`compose/briefseam.go` reads "one queue rather than two readings of it"; `compose/importseam.go`
"delegates rather than reimplementing". If no seam exists for what you need, write the seam.

**Never hand-type a SQL placeholder.** Derive `$N` from the argument slice (`args = append(args, v)`
then `fmt.Sprintf("%s = $%d", col, len(args))`, as `deals/offer_lines.go` does) or use
`storekit.InsertFragments`. Nothing in this repo checks that a statement's column, placeholder and
argument counts agree. A hand-numbered statement shipped in `contacts/researchclaim.go`, and the
accept path was dead for two days because no test executed it.

The `%s` there is the column, and it has its own rule: a compile-time literal, or a catalog name
quoted with `pgx.Identifier.Sanitize` (`storekit/customcolumns.go`). Never a string off a request
body. Values are always `$N`; only identifiers are formatted, and an identifier a caller chose is an
injection.

**A gate that hard-codes its subject copies it**; see [derive the
obligation](derive-the-obligation.md#writing-a-gate-that-holds). Derive the gate's corpus from the
owner it protects (`freemail.Domains()`, the contract, the tree) or say in the test why you cannot.

## What to do with a finding

Pick one of these outcomes explicitly, in order of preference; an unnamed outcome lets a duplicate
survive review.

1. **Adopt.** One implementation wins; the others call it and are deleted. Best when both live in
   the same import tier.
2. **Seam.** The two live in tiers that cannot import each other (a module may not import a sibling
   or `compose`), so `compose` injects the edge: one function crossing, named for the question it
   answers.
3. **Declare the divergence.** The two are different capabilities that read alike. Say so in the
   code, at both sites, naming what makes them different. A reason in the pull request does not
   count, because the next reader is in the file.
4. **Gate it.** Whatever you chose, leave a fitness function so the answer holds without a human
   remembering it. See below.

## Leaving a gate behind

A rule with no gate does not hold here: the frontend rulebook asks authors to open the catalog
first, every time, and the tree still grew a third spelling of a card. The house shape for a reuse
gate follows `agenttoolparity_test.go` and `tableownership_test.go`: derive the expectation from the
tree, keep the escape hatch at the subject, ratify the instance, write the defect test first, and
plant the shape the gate cannot see. The method is in [derive the
obligation](derive-the-obligation.md#writing-a-gate-that-holds).

### A gate is code, so it duplicates like code

**Porting half a rule splits it.** Several of this tree's design gates scan TypeScript *and* CSS.
Moving the TypeScript arm to an AST test and leaving the CSS arm in shell would leave one rule with
two implementations that nothing forces to be edited together. Either both arms move or neither
does; the rule and its case corpus stay singular, and only the parser may differ.

**Extract the shared walk at the second gate.** The first has nothing to compare against; by the
third there are already three answers. Two source-scanning gates once each owned a copy of the
extension walk, and they disagreed about which files a bundler resolves, so a unit was gated by one
and invisible to the other. A gate with a narrower walk reads a smaller tree and still reports PASS,
with no failing assertion to notice. The walk now lives in `frontend/scripts/lib/source-tree.ts`,
with its test beside it. Separate compiler calls also disagreed about the dialect and read a `.ts`
file as TSX, so `parseSource` and `sourceFileAt` in the same file are the one parser, and its test
fails a second call site by name.

## What this does not ask for

- **Not deduplication for its own sake.** Two similar-looking things that answer different questions
  stay two things, with the difference written down. Merging them is how a gate loses the case it
  existed to catch.
- **Not abstraction ahead of a second caller.** The choke point exists because two callers need one
  answer today, not because a third might tomorrow. An abstraction with one caller is a craft
  finding (`T3 premature-abstraction`).
- **Not a rewrite.** The cheapest correct outcome is usually a one-function swap or one seam file.
