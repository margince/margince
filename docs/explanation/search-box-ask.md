# The search box asks the handbook

What a user sees is in the handbook page [What Margince is](../handbook/what-margince-is.md), under
"Where is the handbook, and how do I ask it a question?". Below: how the box tells a question from
a lookup, which document set answers, and how a handbook answer names its section and settings page.

```
 text in "Search or ask Margince"
        |
        +--> record search, screen and setting rows      (as you type, no model)
        |
        +--> looks like a question?  --no-->  box works as a lookup only
                 | yes, and no screen or setting row matched,
                 | and the reader may read a document set
                 v
        row "Ask Margince: ..."   --Enter-->   ask
                                             |
                       +---------------------+
                       v
             1. shipped handbook set   --answered-->  show it
                       | not_covered, or the ask failed
                       v
             2. workspace default set  --answered-->  "Not in the handbook. From <set>"
                       | not_covered                  --failed--> "Question not answered"
                       v
             "Margince answers from its handbook and your documents. ..."
```

## Typing never calls a model

Each keystroke runs the record search, which was there before the ask. The question test is
a word rule, so the box costs no model call
until a user presses Enter on the ask row. A model call per keystroke would bill every name lookup.

The rule wants at least three words, so a name such as "Wo Holdings" stays a lookup. It then wants
a closing question mark, an opening question word ("how", "wie"), or an auxiliary with a subject
after it ("can I", "kann man"). The subject test keeps "Can Do Logistics" a company name.
Vietnamese is matched by two-word phrases at the start or end ("làm sao", "thế nào"), because the
single words are also given names. The word lists are read whatever the screen's language, since a
reader asks in the language they think in.

The ask row shows only when no screen or setting row matched. A question that also matches a
screen keeps the Enter it had before, and can still be asked from **Ask your documents**.

## Two sets, asked in order

The box asks the shipped handbook first, then the workspace's default set (`default_ask`). A set that is both is asked once.
Each ask is a call to `POST /knowledge/corpora/{id}/ask`, the same path **Ask your documents** uses.
Both surfaces send it through one function and read the list of sets through one query, so they
cannot drift.

The second set is asked only when the first answers `not_covered` or its ask fails. Any other
outcome stands. `not_ready` means the set is still being read, and says nothing about the question.
When the handbook answered `not_covered` and the default set's ask then fails, the box shows
"Question not answered". Showing the handbook's answer there would claim the company's documents
were searched too.

## When a set says not covered

An ask has two places that can answer `not_covered`:

- **Before the model.** The question is embedded and passages are ranked by cosine. Passages under
  the grounding floor are dropped. When none is left, the answer is `not_covered` with
  `generated_by: deterministic`, and no writer runs. The floor is a fact about the embedding model,
  measured once per model and kept in a registry in the code. A model with no measured floor gets
  none.
- **In the writer.** When passages pass the floor, the writing model reads them and may still say
  they do not cover the question. Ranking cannot settle that, because cosine is not calibrated.

The handbook is written in English, and the question is not translated before it is embedded. A
question in German or Vietnamese often ranks under the floor against English passages, and gets
`not_covered` from the first place. The answer is written in the base language.

## A handbook claim names its section and settings page

When the answering set is the shipped one, the server adds two fields to each claim with a line
number:

- `section {title, line}`: the `##` or `###` heading the claim's lines sit under.
- `opens {settings_page}`: the page an `<!-- opens: settings/<id> -->` line under that heading
  names.

The server reads the page from this binary's own copy, which is the copy the shipped set was filed from. A span that
opens on a heading belongs to that heading. A span that starts in the tail of one section and runs
into the next heading belongs to the later one, because the chunker cuts a passage wherever it likes.
Uploaded sets get neither field: their pages are not in the binary.

The answer shows **Open Settings → …** only when the reader may open that page, by the same check
the settings navigation uses. The marker rule for handbook authors is in
[update-the-handbook.md](../how-to/update-the-handbook.md). A frontend test fails on a marker whose
id the settings catalog does not list.

## Where the code lives

| Part | Path |
|---|---|
| Question test | `frontend/src/lib/question.ts` |
| Ask row, answer panel | `frontend/src/app/paletteanswer.tsx` |
| Set order and fallback | `frontend/src/features/voice-knowledge/corpusaskqueries.ts` |
| Ask endpoint and its outcomes | `backend/internal/modules/knowledge/ask.go` |
| Grounding floor per embedding model | `backend/internal/modules/knowledge/groundingfloor.go` |
| Section and marker reader | `backend/internal/modules/knowledge/handbook/section.go` |
| Stamping claims | `backend/internal/compose/corpusaskhandbook.go` |
| Marker check against the settings catalog | `frontend/src/features/settings/handbookopens.test.ts` |

## Where to go next

- [search-and-retrieval.md](search-and-retrieval.md): how record search finds records, and the
  embedding identity the floor keys on.
- [ai-runtime.md](ai-runtime.md): where the embedding and writing models come from.
