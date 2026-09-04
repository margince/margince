# One source of truth

**Every topic in this tree has exactly one place that decides it — and module
boundaries decide where that place may live.** Those are two halves of one rule:
no second implementation, and no copy smuggled across a tier because importing
the owner was inconvenient.

"Source of truth" here means the code that DECIDES, not the column that stores.
The authoritative row is a separate question; this principle is about the one
function, constant, statement or seam every caller of a topic passes through.

The rulebook states the obligation in
*Reuse before you build*.
This page is the **method**: how to find out whether a topic already has an
owner, what to do when it has two, and how to leave a gate behind so the second
one cannot come back. Read it before adding a capability, and run its scan when
auditing a subsystem.

## What counts as a topic

A topic is whatever a reader would name in one phrase: "how a person's name is
normalized for dedupe", "how weighted pipeline value is computed", "what a
retention anonymize scrubs", "how we build a system prompt for correspondence",
"how we fetch a URL the tenant supplied". If you cannot name it in a phrase, you
are probably looking at two topics.

Two implementations of one topic is not untidy. It is two answers to one
question, and nothing forces the two to be asked together, so they drift until
they disagree in front of a user. The cost is never paid on the day the second
one is written — it is paid by the third author, who greps, finds one of them,
and does not know the other exists.

A choke point is not a layer, not a base class and not an abstraction. It is one
named function, one SQL literal, one constant, or one seam file that every
caller of that topic goes through.

## Topic kinds and where their choke point lives

The tree already places these. Use the table to find the existing one before
writing anything.

| Topic kind | The choke point | Where |
|---|---|---|
| Writing a domain row | the module's store method, and `storekit.Audit`/`Emit` inside one tx | `modules/<name>/`, `platform/database/storekit` |
| Reading one record's world | `person360.Service.Assemble` / `org360.Service.Assemble`, reached through the `Assembler` interface each consumer declares | `compose/person360`, `compose/org360` |
| An MCP tool that answers a question a page already answers | a `compose/*seam*.go` file | `internal/compose/` |
| A model system prompt for correspondence | `draftrules.Shared` | `compose/draftrules` |
| Fetching a tenant-supplied URL | `platform/webread` (SSRF-guarded via `platform/netguard`) | `platform/webread` |
| A filter/predicate vocabulary | `storekit.FilterSet` / `predicate.go` | `platform/database/storekit` |
| Keyset pagination | `storekit.EncodeCursor`/`DecodeCursor`/`Page` | `platform/database/storekit` |
| An interactive control in the UI | the design system | `frontend/src/design-system/` |
| A sentinel error a client can see | `shared/apperrors` | `internal/shared/apperrors` |
| An outbound event | `event_outbox` via `storekit.Emit`, shipped by `platform/events.Relay` | never a direct XADD |

If your topic is not in the table, that does not mean it has no choke point. It
means you have to run the scan.

## The boundary half

Where the owner is allowed to live is not a style question — it is what makes
the rule enforceable. The DAG is `shared → platform → modules → compose → cmd`
(ADR-0054), and it has one consequence that produces most of this tree's
duplicates:

**A module may not import a sibling module, and may not import `compose`.**

So when capability A in one module needs what capability B in another already
decides, the honest path is always more work than copying:

| A needs B, where B lives in | The sanctioned move |
|---|---|
| `shared/` or `platform/` | **Import it.** No seam needed — that is what those tiers are for. |
| a sibling module | **A seam.** `compose` injects the edge as one named function. |
| `compose/` (a subpackage) | **A seam.** Same shape; `compose` binds its own subpackage to the module's injected interface. |
| the same module | **Call it.** If it is unexported and you need it elsewhere in the package, that is not a boundary problem. |

Copying is never on this list. The reason it keeps happening is that a seam
looks like ceremony next to a fifteen-line copy — and the seam is still the
right answer, every time. Say so out loud in review; it is the only thing that
helps.

Two corollaries worth stating because both have been violated here:

- **A module writes only the tables it owns**, declared in its `doc.go` and held
  by `backend/gates/tableownership_test.go`. A write into a sibling's table is a
  boundary crossing wearing a SQL statement's clothes.
- **A gate keyed by package cannot see an intra-package duplicate.** The
  ownership test keys its waivers `<package>:<table>`, so every second writer
  living inside one module is invisible to it. Boundary gates and duplication
  gates catch different things; you need both.

## The scan — six probes, in this order

Cheap first, and each probe finds a class the previous one cannot.

### 1. Grep the topic's nouns tree-wide, never your directory

```shell
git grep -n "<noun>" origin/main -- backend frontend/src extensions
```

Two rules that decide whether this probe works at all:

- **Grep `origin/main`, not the checkout.** A shared or stale worktree makes an
  absent file look like a refuted premise.
- **Grep the whole tree.** The duplicate is almost never in the package you are
  editing — that is precisely why it was missed.

Grep for the noun in three registers, because the duplicate rarely reuses your
spelling: the domain word ("brief", "weighted", "anonymize"), the mechanism
("normalize", "Casefold", "ToLower"), and the artifact ("`_profile_field`",
"system prompt", "Badge").

**Then ask whether the other language carries a spelling of it too.** Usually it
does not — one side computes and the other renders, which is the shape to want.
But where both DO decide the same rule, the two halves are one topic rather than
two findings on two tiers, because errors on opposite sides of a wire can CANCEL:
each half looks correct in place, and fixing one half is a regression. The
money-scale case is written up in
[find the other side](derive-the-obligation.md#find-the-other-side-before-you-fix-this-one).

### 2. Count the writers of each table

For a topic that ends in a row, the census is exact:

```shell
git grep -nE "(INSERT INTO|UPDATE|DELETE FROM) <table>" origin/main \
  -- backend/internal/modules backend/internal/compose | grep -v _test
```

Include `DELETE`. A table whose rows one package writes and another package
destroys has two owners, not one, and a census of inserts and updates alone
reports it as clean.

More than one non-test writer is a finding to *rule on*, not automatically a
defect — each writer may be a distinct verb. The question to answer in the code
is: **does a new rule about this table have one place to land, or N?** Record
the answer next to the writers.

### 3. Count the importers of each capability package

A capability with exactly one non-test importer serves one surface. Sometimes
that is correct (an internal helper of one package); sometimes it is the finding
(the other surface built its own).

```shell
git grep -l "internal/compose/<pkg>\"" origin/main -- 'backend/*.go' | grep -v _test
```

`compose/meetingbrief` had exactly one importer — the HTTP handler set — while
`prep_for_meeting` assembled its own brief. One number, one finding.

### 4. Read the uniqueness claims and disbelieve them

Grep the tree for prose that asserts a choke point:

```shell
git grep -n "the one spelling\|the only writer\|cannot drift\|the same .* the .* performs\|there is no other" origin/main -- backend frontend
```

**Nine of the ten claims counted exhaustively in this tree were false.** Of the
claims spot-checked rather than counted, several were true but held by no test,
which is the next-worst state: correct today, with nothing to notice when it
stops being. The claim is where the
duplicate hides, because the next author reads it and stops looking. For each hit, run probe 1 or 2 against the thing it claims. A claim no
test holds is either deleted or gated — never left standing.

### 5. Diff the two halves you already know about

When a topic legitimately has two implementations (a SQL expression and its Go
twin; an Art. 17 erasure and a retention anonymize; a website assembler and a
tool assembler), do not read them for similarity — **enumerate what each one
touches and diff the lists**. Reference counts are the cheap version:

```shell
grep -c "field_provenance" erasure.go retentionactions.go
```

A zero on one side of a pair is the whole finding. Similarity reading finds
nothing; counting does.

### 6. Ask what a sweep would have to edit

The last probe is a thought experiment with a mechanical answer: *if a rule about
this topic changed tomorrow, how many files would the change have to touch?* If
the answer is more than one, either the topic has no choke point yet — whatever
the comments say — or it has a **declared divergence**, which is the legitimate
multi-site case. The difference is whether each site says, in the code, what
makes it different. An undeclared N is the finding; a declared N is a decision
somebody already made and wrote down. This is the probe that found `person_profile_field`'s five writers
and `relationship`'s four.

## The rules, and what each one cost

The rulebook states these as imperatives. Here is the incident behind each, since
a rule whose price nobody remembers is a rule that gets argued away.

**Search the whole tree, not your directory.** The duplicate is almost never in
the package you are editing — that is precisely why it gets missed. The agent tool
`prep_for_meeting` was written beside a working `compose/meetingbrief/` that a
one-word grep would have found, and the two answered one question with different
grounding rules until a seam was written. Grep the nouns across `backend/`,
`frontend/src/` and `extensions/`, in three registers: the domain word, the
mechanism, the artifact.

**The tool surface and the web surface share ONE engine.** An MCP tool never
re-derives what an HTTP handler already computes. The seams that exist state the
rule in their own words — `compose/briefseam.go`: "one queue rather than two
readings of it"; `compose/importseam.go`: "it delegates rather than
reimplementing, and that is the whole design". If no seam exists for what you
need, **write the seam**. A module may not import a sibling or `compose`
(ADR-0054 §3), so rolling your own always looks like the cheaper path, and it is
always the wrong one.

**Never hand-type a SQL placeholder.** Derive `$N` from the argument slice —
`args = append(args, v)` then `fmt.Sprintf("%s = $%d", col, len(args))`, as
`deals/offer_lines.go` does — or use `storekit.InsertFragments`. Nothing in this
repo checks that a statement's column count, placeholder count and argument count
agree, so a hand-numbered statement is one careless sweep away from binding every
column to the wrong value. It shipped in `people/researchclaim.go`, and the accept
path was dead for two days because no test executed the statement.

The `%s` there is the COLUMN, and it carries its own rule: a compile-time
literal, or a catalog name quoted with `pgx.Identifier.Sanitize` — the one
spelling this repo uses (`storekit/customcolumns.go`). Never a string off a
request body. Values are always `$N`; only identifiers are ever formatted, and an
identifier a caller chose is an injection with a placeholder's manners.

**A gate that hard-codes any part of its subject has become a second copy of
it.** A census over consumer-mail domains carrying its own sample of them, a
design gate restating the element list it forbids, a parity test with a
hand-maintained coverage map — each is the duplicate it was written to refuse, and
it goes quietly short the day somebody extends the owner. Derive the gate's corpus
from the owner it protects (`freemail.Domains()`, the contract, the tree) or say
in the test why you cannot. Every gate the duplication sweep produced was found,
after shipping and by a reviewer, to have hard-coded part of its own subject; two
reviewers independently named the domain sample a consumer-mail gate kept inside
the test forbidding second consumer-mail lists.

## What to do with a finding

Four outcomes, in order of preference. Pick one explicitly — leaving it unnamed
is how a duplicate survives a review.

1. **Adopt.** One implementation wins; the others call it and are deleted. Best
   when both live in the same import tier.
2. **Seam.** The two live in tiers that cannot import each other (a module may
   not import a sibling or `compose`, ADR-0054 §3), so `compose` injects the
   edge — one function crossing, named for the question it answers. **Write the
   seam; do not write a second assembler.** Rolling your own always looks
   cheaper, and it is always wrong.
3. **Declare the divergence.** The two are genuinely different capabilities that
   read alike. Say so in the code, at both sites, naming what makes them
   different. A reason in the pull request does not count — the next reader is
   in the file, not the PR.
4. **Gate it.** Whatever you chose, leave a fitness function so the answer holds
   without a human remembering it. See below.

Two writers of one invariant either share a helper or say why they do not. If
you are adding the second, the reason goes in the code beside it.

## Leaving a gate behind

A rule with no gate has already been tried here and already failed: the frontend
rulebook has said "open the catalog first — every time, not once" for months,
and the tree still grew a third spelling of a card.

The house shape for a reuse gate, learned from `agenttoolparity_test.go` and
`tableownership_test.go`:

- **Derive the expectation from the tree, never from a list in the test.** A
  hardcoded coverage set is a list to maintain, and the rulebook's *Rules
  learned from the review loop* #2 says prefer the fitness function. `draftrulesparity_test.go` hardcodes four
  drafting surfaces; a sweep found at least one more that carries none of the
  shared rules and is invisible to it. The exact count is not the point and I am
  deliberately not quoting one — the failure mode is that a hardcoded census
  cannot notice the surface added after it was written.
- **No waiver map in the test.** The escape hatch lives at the subject — a
  `doc.go` line, a contract field, a `//craft:ignore <check> <reason>` — where a
  reviewer editing that code sees it. A map in the test is invisible to the
  author who needs it.
- **Ratify the instance, never the category.** A waiver keyed by column, by
  package, or by rule name lets a second offender in for free under the first
  one's reason.
- **Write the defect test first.** Mutate the code to reintroduce the duplicate
  and watch the gate go red. A gate that has never failed proves nothing; four
  fitness functions in this tree passed against the exact bug they described.
- **Measure the before, not only the after.** A grep that returns zero after the
  fix proves nothing unless you know it returned N before.
- **Derive the gate's own corpus too, not just its file list.** A gate that
  hard-codes any slice of what it guards has become a second copy of it — and a
  copy living inside a test is invisible to the author extending the owner. Ask
  what your gate hard-codes before the first assertion.
- **Plant the shape the gate cannot see, once it is green.** One mutation proves
  it fires; the second finds the hole. The full method, and the failures behind
  each clause, is in
  [derive the obligation](derive-the-obligation.md#writing-a-gate-that-holds).

### A gate is code, so it duplicates like code

Two traps, both paid for here, both of them this principle's own subject wearing
the fix's clothes:

**Porting half a rule puts one rule in two homes.** Five of this tree's design
gates scan TypeScript *and* CSS. Moving the TypeScript arm to an AST test and
leaving the CSS arm in shell would leave one rule with two implementations that
nothing forces to be edited together — which is the thing this page exists to
refuse. Either both arms move or neither does; what stays singular is the rule
and its case corpus, and only the parser may differ.

**The SECOND gate of a kind is when to extract the shared traversal.** The first
has nothing to compare against; by the third there are already three answers.
Two source-scanning gates here each owned a copy of the extension walk and had
already drifted before either was reviewed — they disagreed about which files a
bundler resolves, four extensions against eight, so a unit was gated by one and
invisible to the other for no reason either author chose. A walk is the worst
place for a second answer: the gate with the narrower walk reads a smaller tree
and reports the same word for it, PASS, and there is no failing assertion to
notice. The one answer is now `frontend/scripts/lib/source-tree.ts`, with its
test beside it rather than inside either caller.

## What this does not ask for

- **Not deduplication for its own sake.** Two similar-looking things that answer
  different questions stay two things, with the difference written down. Merging
  them is how a gate loses the case it existed to catch.
- **Not abstraction ahead of a second caller.** The choke point exists because
  two callers need one answer today, not because a third might tomorrow. An
  abstraction with one caller is a craft finding (`T3 premature-abstraction`).
- **Not a rewrite.** The cheapest correct outcome is usually a one-function swap
  or one seam file, and the audit that produced this page found exactly that.
