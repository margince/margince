# UI copy: voice, tone and grammar

English is the source catalog. Every German and Vietnamese string is translated
from `frontend/src/i18n/en.ts`, so a defect in the English text becomes three.
This page is the standard the English catalog is held to.

Part of it is mechanical and held by `frontend/src/i18n/copy-style.test.ts`,
which fails on any `en` value that breaks one of those rules and names the key.
[What the gate holds](#what-the-gate-holds) lists them. Everything else here is
the author's judgement, and a reviewer reads a copy change against this page.

Margince is a B2B CRM for sales teams and their management. The reader is at
work, usually mid-task, often scanning a list or a report. Copy exists to let
them act, not to be read for its own sake.

## Voice

Margince speaks as a professional tool: not as a colleague, not as a mascot.

- **Plain.** Say what a thing is in the words a sales professional already uses.
  No coined terms for established functions: a tab is a tab, a filter is a
  filter, a search is a search.
- **Precise.** Every label describes the thing beside it. A hint says what the
  field takes. A status says what is true now. Never approximate.
- **Brief.** The shortest sentence that carries the fact. No filler, no
  preamble, no reassurance, no explanation of why the reader might want this.
- **Neutral.** The product reports. It does not chat, apologize, celebrate or
  joke. No exclamation marks. No "just", "simply", "please note", "feel free",
  "go ahead".
- **Confident.** State what happened and what to do. No hedging words such as
  "might", "perhaps" or "try to", unless the uncertainty is real and material.

### Address

Copy addresses nobody by default. Labels, titles, statuses, table headers, menu
items and most help text are impersonal statements.

- Use "you" and "your" only where the sentence would otherwise be ambiguous
  about ownership ("Your drafts" beside "Team drafts"), or where an error or
  confirmation tells the reader what to do next ("Sign in again to continue").
- Never "we", "us" or "our". Margince is software. It has no opinions,
  intentions or feelings. "We could not save" becomes "Changes were not saved".
  The one exception is the privacy notice, where the data controller addresses
  the data subject and in law speaks as "we".
- Never "I", with one exception: a conversation surface where an agent
  literally speaks in a message bubble (the onboarding conversation). There the
  agent says "I" in short, factual sentences, and nowhere else. Status lines,
  notifications and activity entries about agent work use the neutral voice:
  "Summary of {name} ready", not "My summary of {name} is ready".
- "Me" is allowed as the object of a control that picks the reader: "Assign to
  me", "Only me". It is a conventional UI label, not the product speaking.
- "My" is allowed only as a scope label that names the reader's own records
  ("My deals"), never inside a sentence.
- A consent statement the reader makes as their own words (an acknowledgement
  before sending, a consent wording) is the reader's sentence, so it may say
  "I" and "we" and is exempt from both pronoun rules.
- Never "please".

### Tone by situation

| Situation | Tone | Example |
|---|---|---|
| Label, heading, menu item | Noun or noun phrase, no verb, no article | Close date |
| Button | Verb, or verb and object, no article | Save changes |
| Status | Adjective or past participle | Sent, Overdue, Awaiting approval |
| Empty state | One fact, one action | No deals yet. Create a deal to start a pipeline. |
| Help or hint text | One sentence stating what the field takes or what the control does | Whole tokens, 1 to 1,000,000,000,000. |
| Success | Past tense, no celebration | Stage added |
| Error | What happened, then what to do | Changes were not saved. Check the connection and retry. |
| Warning | What will happen, then what is at stake | Deleting this stage moves its deals to the previous stage. |
| Confirmation | Title asks; primary button names the verb | Delete stage? / Delete stage |
| Legal, privacy, consent | Exact; every qualifier kept | Designed to support compliance |

## Grammar and mechanics

- **American English** spelling: color, canceled, license (noun
  and verb), catalog, analyze, center.
- **Sentence case** everywhere: titles, headings, labels, buttons, menu items,
  tabs, badges. Capitalize proper nouns and product names only.
- **No articles** in labels, buttons, headings and menu items: "Create
  password", not "Create a password".
- **Periods** end complete sentences in help text, messages and notifications.
  No period on labels, headings, buttons, tooltips, badges, menu items or
  single-fragment hints.
- **No contractions.** "Cannot", "did not", "is not".
- **Present tense** for what is true, **past tense** for what completed
  ("Upload failed", "Stage added"). No future tense for immediate effects.
- **Active voice** where the actor matters. Passive is acceptable when the
  actor is the system and irrelevant ("Changes were not saved").
- **Curly apostrophes and quotes** in visible text: ’ “ ”. Never a straight
  ' or " inside a value.
- **No dashes as punctuation.** No em dash, no en dash, no spaced hyphen as a
  sentence break. Rewrite with a period, a colon, a comma or parentheses. The
  hyphen only joins compound modifiers: "read-only", "per-user".
- **Ellipsis** (…, one character) only on a state in progress: "Saving…".
  Never as trailing suspense in prose.
- **No exclamation marks.**
- **Spacing.** One space between words, never two. A value may begin or end
  with a space only when it is a fragment the code joins around markup, where
  that edge space is load-bearing.
- **Numbers** as digits, with a comma thousands separator. "to" for ranges
  ("1 to 4", never "1-4"), "of" for part of a set ("3 of 12"). No ordinals in
  UI (1st, 2nd).
- **Abbreviations**: none of e.g., i.e., etc., vs., w/ or &. Write "for
  example", "that is", "and". Established acronyms are fine when the reader
  shares them: CRM, API, VAT, DNS, IMAP, PDF, CSV, UTC.
- **Lists** keep parallel form. Fragments are lowercase with no end
  punctuation.
- **Pronouns** for a human being are they/them.
- **Dates and times** come from the formatting layer and are never spelled in a
  string. Relative time words follow the pattern "just now", "{n} minutes ago",
  "yesterday", "in {n} days".
- **Placeholders** (`{name}`, `{count}`) are never renamed, dropped or added.
  The text around a placeholder must read correctly for every value the code
  can supply; a count that can be 0 or 1 needs plural arms, not "(s)".

## Length ceilings

| Kind | Ceiling |
|---|---|
| Button, label, menu item, tab, badge | 3 words |
| Title, heading, dialog title | 6 words |
| Hint, help text, tooltip | 1 sentence, 90 characters |
| Message body (error, warning, info, empty state) | 2 sentences, 180 characters |
| Legal, privacy, consent, license notices | Exempt from length; every qualifier preserved |

A string over its ceiling is not wrong by definition, but its author says why.

## Vocabulary

One word per concept, the same word on every screen. Product names are proper
nouns and are not reworded. Internal jargon never reaches the screen.

| Concept | Write | Never |
|---|---|---|
| The tenant | company | workspace, account, the retired company nouns in [record-vocabulary.md](record-vocabulary.md) |
| A human record | contact | the retired record nouns in [record-vocabulary.md](record-vocabulary.md), except where the word means a human being rather than the record |
| A company record | company | account and the other retired record nouns in [record-vocabulary.md](record-vocabulary.md) |
| Sales object | deal | opportunity |
| Ordered stages | pipeline | funnel |
| Manager's forecast number | call | commit (which is a per-deal forecast category) |
| The reader's queue | Worklist | queue, inbox |
| Daily digest | Morning brief | briefing, digest |
| Agent decisions awaiting a human | Approvals | decisions, verdicts |
| Agent credential | passport | token, key (except API keys) |
| Mail or calendar link | connector | integration |
| Buyer-facing deal page | Deal Room | portal |
| The mail thread of a record | thread | spine, conversation |
| Import of mailbox history | mailbox history import | backread |
| Full read of a web page | full page read | deep read |
| A classifier's result | result, check | verdict |
| Capture intake step | intake check | admission check |
| Loading a record | Loading… | Reading… |
| Ownership of a deal | owns | carries |
| Tabs of a record | (tab labels only) | Parts of this record |

## Message shapes

**Error.** First sentence: what did not happen, in the past tense, without
blame. Second sentence: the one action that resolves it. Never a stack trace,
an internal name or a table name. "Changes were not saved. Retry, or reload the
page."

**Warning.** What will happen if the reader proceeds, and what it costs.
"Removing this stage moves 12 deals to Qualification."

**Confirmation dialog.** The title is the question: it ends with a question
mark and names the object, as in "Delete stage?" The body states the
consequence in one sentence. The primary button repeats the verb and object:
"Delete stage". The secondary button is "Cancel". Never "Yes", "No", "OK" or
"Confirm".

**Empty state.** One sentence of fact, then one action if there is one. "No
contacts match these filters." "No deals yet. Create the first deal."

**Success.** Past participle, no period, no celebration: "Stage added",
"Invitation sent". It names the object, not the feeling.

**Permission denied.** The fact and who can change it: "Only an administrator
can edit the allowance."

**Withheld or partial.** What is missing and why, in one sentence: "Amounts are
hidden for this role."

**In progress.** Present participle with ellipsis: "Saving…", "Loading deals…".

## What the gate holds

`frontend/src/i18n/copy-style.test.ts` checks every value of the `en` catalog,
one test for each of its twelve rules, and lists each offender as its key and value:

| Rule | What fails |
|---|---|
| No dashes | An em dash or an en dash |
| No exclamation marks | Any `!` |
| Curly apostrophes and quotes | A straight apostrophe between two letters, or any straight double quote |
| Ellipsis | Three dots in a row instead of … |
| Spacing | Two spaces in a row |
| Abbreviations | e.g., i.e., etc., vs. |
| Never "please" | The word, in any case |
| No contractions | A fixed list: can’t, won’t, don’t, isn’t, aren’t, didn’t, doesn’t, couldn’t, wasn’t, hasn’t, haven’t, it’s, that’s, there’s, you’re, you’ve, we’re, they’re, let’s, with either apostrophe |
| Never "we" | we, us, our, ours in lower or leading capital (so the country code "US" passes), outside keys starting `privacynotice.` or `prefs.wording.` and the consent statements listed in the test |
| Never "I" | I, I’m, I’ve, myself, outside keys starting `ob.conv.` or `prefs.wording.` and the consent statements listed in the test |
| No ampersand | Any `&` outside curly quotes; write "and". A label in “…” is copied from another product's screen and must match it |
| American spelling | A fixed list of British spellings of words the catalog uses |

Placeholders are removed before the word rules run, so `{name}` never reads as
copy. The gate cannot see the rest of this page: tone, sentence case, articles,
periods, tense, voice, a spaced hyphen, w/, numbers and ranges, list form,
length ceilings, vocabulary, message shapes, a contraction outside its list, and
"you" used where nobody needed addressing. Those are the author's and the
reviewer's judgement.

## Sources

The grammar and message rules follow the Atlassian Design System content
guidelines (voice and tone, language and grammar, inclusive writing, date and
time, designing messages), adjusted where Margince is more formal: no
contractions, a product that never says "I" or "we", no humor.
