# Update the handbook

The handbook in `docs/handbook/` is not only reading matter. Every build embeds
it, every boot files it as the **Margince handbook** document set, and that set
is the default behind **Ask your documents** (⌘K). A page you write is therefore
the answer a user gets to a question — or the reason they get **Not covered by
this set**. Write it for the machine that reads it, and measure it before and
after.

## How the ask reads a page

Four facts decide whether a paragraph can ever be an answer. The code is the
authority; these are where it lives.

1. **A page is cut into passages of at most 800 characters**, preferring a
   blank line, then `. `, then a newline, with 120 characters of overlap
   (`backend/internal/modules/knowledge/chunk.go`). A heading is **not** carried
   into the passages below it, so a passage that starts mid-section has to
   explain itself.
2. **The question is embedded and the 8 nearest passages are ranked**; anything
   under the corpus's similarity floor is dropped
   (`backend/internal/modules/knowledge/ask.go`). A user's words have to be near
   the passage's words — a synonym that appears nowhere cannot be found.
3. **A writer model decides whether those passages answer the question**
   (`backend/internal/compose/corpusask.go`). Its prompt is strict on purpose:
   *a question asking HOW to do something is not answered by a passage saying
   what the thing IS, when it comes into being, who may do it, or what happens
   afterwards.* Concept prose, however accurate, gets `not_covered` for every
   "how do I…".
4. **Every claim must quote its passage verbatim.** A claim whose quote is not
   in the passage is dropped; an answer with none left is `not_covered`.

## The principles

- **Every task a user can do has a task section.** `### How do I <verb>
  <noun>?`, in the words a user types.
- **The first sentence is the whole answer** and names the noun and the screen:
  "To create a deal in Margince, open **Deals** in the sidebar and choose
  **New deal**." Numbered steps follow, with the exact visible labels in bold.
- **One task, one passage.** Keep a task section under about 650 characters and
  put no blank line inside it — not even under its heading — so it lands whole
  in one passage.
- **Synonyms are written, not assumed.** End a task with `Also called: …`
  (opportunity, account, proposal, teammate, "the customer said no"), and
  keep the glossary in `what-margince-is.md` current.
- **A task Margince cannot do still gets a section.** "You cannot create an
  invoice in Margince; invoices come from …" is an answer. Silence is
  `not_covered`, which a user reads as the product being broken.
- **Concept sections name their subject in their first sentence** ("The
  Worklist is …", never "It is …"), for the same reason as above: the heading is
  not in the passage.
- **Every label, number and rule is checked in the code** — the English catalog
  `frontend/src/i18n/en.ts` and the screen that renders it, and `backend/` for
  limits and refusals. A handbook that names a button that is not there is worse
  than one that says nothing, because the answer cites it.
- **Two record words are gated.** The record is a *contact* and never its
  retired name (`backend/gates/contactvocabulary_test.go`; write someone,
  colleagues or users instead), and a *company* is never called by its retired
  name outside the glossary in `what-margince-is.md`,
  which `backend/gates/companyvocabulary_test.go` waives by name because a
  synonym nobody writes cannot be retrieved. Put such a word in the glossary,
  not on a task page.
- **Links stay inside `docs/handbook/`.** The folder is a self-contained corpus;
  a link out of it resolves nowhere once uploaded.

## The procedure

1. **Start a stack and measure first.** `make dev`, then `make seed-dev` (in a
   linked worktree, `API_BASE=http://localhost:<api port> ./scripts/seed-dev.sh`;
   the startup line prints the port). Wait for the handbook to finish ingesting,
   then run

   ```sh
   API_BASE=http://localhost:<api port> scripts/handbook-ask/probe.sh \
     scripts/handbook-ask/questions.txt .tmp/before.tsv
   ```

   It prints how many questions came back `answered` and lists the rest with
   their outcome. `unreviewed` is the writer model failing (a provider 503),
   not the page; the probe retries it.
2. **Write a held-out set you do not show yourself while writing** — ten or
   twenty questions in new words about the area you are changing. It is the
   only honest measure of whether you wrote for users or for the bank.
3. **Read the misses.** Each row carries the writer's summary. "Your handbook
   explains what X is but not how to …" means a missing task section; a summary
   about a different subject means a missing synonym or a buried first sentence.
4. **Edit `docs/handbook/`** by the principles above. Check each label in the
   catalog and its screen before you write it.
5. **Embed and rebuild.** `make -C backend handbook-embed` copies the pages
   into the binary's package; the API does not hot-reload, so run `make dev`
   again. The boot re-files only the pages whose bytes changed and re-embeds
   them.
6. **Measure again** with the bank and the held-out set. Every question you
   fixed goes into `scripts/handbook-ask/questions.txt` in the words a user
   would use, so the next change cannot quietly lose it.
7. **Gates, then commit both copies.** `go test -count=1 ./gates/ -run
   'Vocabulary|Reachab|PublicRef'` from `backend/`, and
   `go test ./internal/modules/knowledge/handbook/`, which fails when the
   embedded copy and `docs/handbook/` differ. Commit `docs/handbook/` and
   `backend/internal/modules/knowledge/handbook/` together.

## Checklist

- [ ] Baseline probe run and saved before the first edit.
- [ ] Every task in the area has a `### How do I …?` section whose first
      sentence answers it.
- [ ] No task section over about 650 characters or with a blank line inside.
- [ ] `Also called:` synonyms on every task; the glossary updated for new words.
- [ ] Tasks the product cannot do are answered plainly, with what to do instead.
- [ ] Every bold label found in `frontend/src/i18n/en.ts` and on its screen;
      every number found in `backend/`.
- [ ] Both vocabulary gates green; no link out of `docs/handbook/`.
- [ ] `make -C backend handbook-embed` run, API rebuilt, probe re-run: no
      question that was answered before is now missing.
- [ ] Held-out set measured, and the fixed questions added to the bank.
- [ ] Wrong copy you found in the product fixed in the same change, or filed.
