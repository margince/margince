<!-- prose:plain -->
# Update the handbook

Every build puts the handbook in `docs/handbook/` into the program. Every start files it as the
**Margince handbook** document set. On a new install, that set is the
default behind **Ask your documents** (⌘K). A page you write is the answer a user
gets to a question, or the reason they get **Not covered by this set**. Write it
for the machine that reads it, and measure it before and after.

## How the ask reads a page

These facts decide whether a paragraph can be an answer. The code has the
last word, and each item names where the rule is.

1. **The ask breaks a page into passages** of at most 800 characters. The break comes at a
   blank line first, then at `. `, then at a line break. Passages share 120 characters
   (`backend/internal/modules/knowledge/chunk.go`). A passage does **not** hold the heading
   above it. So a passage that starts part of the way down a section has to
   explain itself.
2. **The question is embedded**, and the 8 closest passages are ranked. The ask drops a passage
   under the similarity floor of the corpus
   (`backend/internal/modules/knowledge/ask.go`). The words of a user have to be close to
   the words of the passage. The ask cannot match a synonym that is on no page.
3. **A writer model decides** whether these passages answer the question
   (`backend/internal/compose/corpusask.go`). Its prompt gives a rule for a question
   that asks how to do something. A passage that says what the thing is does not answer it.
   This also holds for a passage about when the thing starts, who may do it, or what comes after.
   A concept passage gets `not_covered` for every "how do I…", even when it is right.
4. **Every claim quotes its passage word for word.** The ask drops a claim whose quote is not
   in the passage. An answer with no claim still in it is `not_covered`.

## The principles

- **Every task has a task section**: `### How do I <verb> <noun>?`,
  in the words a user types.
- **The first sentence is the whole answer**. It names the thing and the screen:
  "To create a deal in Margince, open **Deals** in the sidebar and choose
  **New deal**." Numbered steps follow. Put the labels in bold, as the screen shows them.
- **One task, one passage.** Keep a task section under about 650 characters, and
  put no blank line in it, not even under its heading. Then it stays whole
  in one passage.
- **Write synonyms out.** The ask finds only the words a page holds. End a task with `Also called: …`
  (`opportunity`, `account`, `proposal`, `teammate`, "the customer said no"). Keep the
  glossary in `what-margince-is.md` current.
- **A task Margince cannot do** still gets a section. "You cannot create an
  invoice in Margince; invoices come from …" is an answer. Silence is
  `not_covered`, and a user reads that as a product that does not work.
- **A concept section names its subject** in its first sentence ("The
  Worklist is …", never "It is …"), because the passage does not hold the heading.
- **Check every label, number and rule** in the code. Labels are in the English catalog
  `frontend/src/i18n/en.ts` and the screen that shows it. Limits, and what the server refuses, are in `backend/`.
  A handbook that names a button that is not there does more harm
  than one that says nothing, because the answer quotes it.
- **Gates check two record words.** The record is a *contact*, and never its
  old name (`backend/gates/contactvocabulary_test.go`; write someone,
  colleague or user instead). A *company* never has its old
  name outside the glossary in `what-margince-is.md`.
  `backend/gates/companyvocabulary_test.go` allows that one page to use it by name, because the ask cannot
  find a synonym that no page has. Put such a word in the glossary, not on a
  task page.
- **Links stay in `docs/handbook/`.** The folder is a corpus that stands on its own.
  After the upload, a link out of it goes to no page.

## The steps

1. **Start a stack and measure first.** Run `make dev`, then `make seed-dev`.

   In a linked worktree, run `API_BASE=http://localhost:<api port> ./scripts/seed-dev.sh`;
   the start line prints the port. Wait until the handbook is in the corpus,
   then run:

   ```sh
   API_BASE=http://localhost:<api port> scripts/handbook-ask/probe.sh \
     scripts/handbook-ask/questions.txt .tmp/handbook-before.tsv
   ```

   It asks the set named **Margince handbook**, not the default set, because an
   admin may have changed the default. Set
   `HANDBOOK_CORPUS_ID` to ask another set. It prints how many questions have the outcome
   `answered`, and it lists the rest with their outcome. `not_covered` is a
   miss that counts. `unreviewed` means the writer model failed (a provider 503), and the page is not
   the problem; the probe tries it again. A run that still ends with an `error`,
   `unreviewed`, `not_ready` or `retrieval_unavailable` row returns an error,
   because the count it reports is short.
2. **Write a second set of questions first.** Do not look at it while you write. Write 10 or
   20 questions, in new words, about the part you change. They show
   whether you wrote for users or for the bank.
3. **Read the misses.** Each row has the summary of the writer model.

   "Your handbook explains what X is but not how to …" means a task section is missing. A summary
   about a different subject means a synonym is missing, or the answer is not in the first sentence.
   To see what the search gave the writer for one question, without a stack,
   run `make ai-eval Q='how can I create a project'`. It ranks the embedded
   handbook with the same chunker and floor as the product. The probe measures the
   whole ask, writer model too; `ai-eval` measures the ranking alone.
4. **Edit `docs/handbook/` by the principles above.** Check each label in the
   catalog and its screen before you write it.
5. **Embed and build again.** `make -C backend handbook-embed` copies the pages
   into the package of the program. The API does not see changes while it runs, so run `make dev`
   again. The start files again only the pages whose content changed, and embeds
   them again.
6. **Measure again with both sets.** Every question you
   fixed goes into `scripts/handbook-ask/questions.txt`, in the words a user
   would use. Then the next change cannot drop it without a sign.
7. **Run the gates, then commit both copies.** From `backend/`, run
   `go test -count=1 ./gates/`. The gates for vocabulary, links you can reach, page length
   and public references are there. `-count=1` is needed. Without it, the Go test cache
   does not run a gate again when only its page changed.

   Then run
   `go test ./internal/modules/knowledge/handbook/`, which fails when the
   embedded copy and `docs/handbook/` are not the same. Commit `docs/handbook/` and
   `backend/internal/modules/knowledge/handbook/` together.

## Checklist

- [ ] A first probe run is saved before the first edit.
- [ ] Every task in the part has a `### How do I …?` section whose first
      sentence answers it.
- [ ] No task section is over about 650 characters or has a blank line in it.
- [ ] Every task has `Also called:` synonyms; the glossary is updated for new words.
- [ ] Tasks the product cannot do have an answer in plain words, with what to do instead.
- [ ] Every bold label is in `frontend/src/i18n/en.ts` and on its screen;
      every number is in `backend/`.
- [ ] Both vocabulary gates are green; no link leaves `docs/handbook/`.
- [ ] `make -C backend handbook-embed` has run, the API is built again, and the probe has run again. No
      question with an answer before is now missing.
- [ ] The second set is measured, and the fixed questions are in the bank.
- [ ] Wrong text you see in the product is fixed in the same change, or has an issue.
