# Docs prose: voice and the house bar

Every Markdown page in this repository is held to this page. The UI catalog has its own standard in
[ui-copy-style.md](ui-copy-style.md); this one covers documentation, rulebooks and READMEs.

`backend/gates/docsprose_test.go` checks the mechanical half on every page and fails on a violation.
[What the gate holds](#what-the-gate-holds) lists those rules. The rest is the author's judgement, and a reviewer
reads a docs change against this page.

## Voice

- **Plain.** Short sentences in ordinary words. Name a thing the way the reader meets it: the screen's label in the
  handbook, the identifier in a reference page.
- **Specific.** A reason names the mechanism and what fails without it. "Retries are capped at five because the
  provider bans a key after ten failures" is a reason. "This keeps things robust" is not.
- **Current.** A page says what is true now. History belongs in git and in [CHANGELOG.md](../../CHANGELOG.md).
- **Checkable.** Every number, path, command and default is one a reader can find in the tree. A count the code owns
  is linked from the page that generates it, never typed by hand.
- **Public.** Name nothing a reader cannot open: no private repository, document, pull request or spec id. State the
  rule in place.

## One job per page

The tree follows [Diátaxis](https://diataxis.fr/). A page does one of four jobs, and a paragraph that serves
another job is cut or moved to the page that owns it.

| Kind | Where | Holds | Leaves out |
|---|---|---|---|
| Tutorial | `docs/tutorials/` | One guided path that works on a fresh checkout | Options, alternatives, reasons |
| How-to | `docs/how-to/` | Steps toward one goal, for a reader who knows the basics | Why the design is this way |
| Reference | `docs/reference/` | Facts to look up: flags, defaults, tables | Narrative, incidents, essays in table cells |
| Explanation | `docs/explanation/`, `docs/principles/` | The reasons behind a design, each one specific | Steps, change history |

The handbook (`docs/handbook/`) and `user-guide/` are for those who use Margince, not those who build it. They use
the words on the screen. Server, transaction, row, token, an API path or a permission key appears only when the
reader must type it. One handbook page owns a topic, and the others link to it.

## What the gate holds

Each rule runs on prose with fenced code, inline code, link targets and HTML comments removed. Tables count as prose.

| Rule | Fails on | Write instead |
|---|---|---|
| `emdash` | More than 3 em dashes per 1,000 words (at least 2 are allowed) | A period, comma, colon or parentheses |
| `lexicon` | honest, genuinely, deliberately, on purpose, quietly, load-bearing, exactly, "is the point", "earns its place" | The plain claim, or nothing: "exactly one" is "one" <!-- prose:allow lexicon the rule quotes the words it bans --> |
| `caps` | An ordinary word in capitals for emphasis (ONE, NOT, OWN) | Lowercase, or *italics* where stress changes the meaning <!-- prose:allow caps the rule quotes its own examples --> |
| `bold` | A bold span over 8 words | Bold for a label the reader clicks or a term where it is defined |
| `negation` | "X is not Y. It is Z." and "not just X, but Y" | State Z <!-- prose:allow negation the rule quotes the shape it bans --> |
| `sentence` | A sentence over 40 words | Two sentences |
| `residue` | "used to be", "previously", "this change", "first cut", "Task N" | The current fact; git holds the history <!-- prose:allow residue the rule quotes the phrases it bans --> |
| `preamble` | "This page explains…" | Start with the content <!-- prose:allow preamble the rule quotes the opener it bans --> |
| `count` | A typed count of 5 or more modules, AI tasks, tools, gates, tables and similar | A link to the page that lists them |
| `leadin` | "Three things:" over a list that is not three long | No number, or the right one |

Capitals for acronyms and runs of SQL keywords (`NOT NULL`, `ON CONFLICT DO UPDATE`) are fine. `CHANGELOG.md`,
`docs/evidence/` and the pull-request template may describe history; every other rule still applies to them.
`CODE_OF_CONDUCT.md` is adopted text and is exempt.

When a banned form must stay, for example a quoted screen label or third-party text, put a waiver on the line above:

```markdown
<!-- prose:allow lexicon the card's own title -->
The card is titled "Honest limits".
```

A waiver names its rule (or several, comma-separated) and gives a reason. It covers its own line and the next, so
a table row can carry one at its end.

## Beyond the gate

Two tests catch what a pattern cannot:

- **The deletion test.** Remove the sentence. If the reader loses no fact, step or reason, leave it out.
- **The any-project test.** Could the sentence sit unchanged in another project's docs? If so, it says nothing about
  this one. Replace it with the specific fact.

Aphorisms fail both. "A gate that can fail short has already failed" sounds like a rule but tells the reader
nothing to do. Write the rule: "A gate must fail when its corpus shrinks; plant a defect and check it fires."

Other gates check other parts of a docs change. `docscodepaths_test.go` checks that named source paths exist.
`docsnamedthings_test.go` checks make targets, environment variables and retired instructions.
`publicreferences_test.go` checks for private references. `docspagelength_test.go` holds the page-length budget.

## Renaming a term

A domain rename changes identifiers, schema and UI copy, but not ordinary English. Before you run a tree-wide
replace, list the prose hits (`git grep -n -w '<old word>' -- '*.md'`) and review each one by hand. "Deal"
becoming "opportunity" is right for the CRM record and wrong for "a good deal of time."
