<!-- prose:plain -->
# The company record page: one gated read, text per viewer, named gaps

The account page a rep opens on a company. It shows who works there, what is open, what moved, and what
to do next. A written brief covers all of it. It is the largest composite surface in the product. Nearly
all of it is put together in `internal/compose`. That is gated section reads inside one transaction
(listed below), plus two groups that work across modules and own view state of their own.

## The shape

More than one endpoint serves one screen. Which one owns which part:

```text
                    the company record screen (companies.tsx)
        ┌──────────────────────────────────────────────────────────┐
        │  header · state strip · health          suggestions card │
        │  contacts · deals · timeline            connections card │
        │  work in flight · facts box             Ask Margince     │
        └──────────────────────────────────────────────────────────┘
             ▲                    ▲                    ▲
 GET /companies/{id}/360   GET …/graph        POST …/ask
   ONE tx, gated per section,    one-hop            per-viewer, prepared
   a refused section is          node/edge set,     questions over the same
   NAMED in sections_omitted     per-group grants   assembly (…/brief too,
                                                    deprecated, no UI)
             │
 POST …/view-ack             advance the visit baseline (explicit, human-only)
 POST …/suggestions/dismiss  per-user, "not this, not now" — a rule's row or a scan's
 POST …/scan                 ensure the reader's account scan is current (human-only)
 GET  …/scan                 the scan as it stands: state, merged advice, the read's foot
 GET  …/logo                 resolved from the site read; monogram floor
```

## One gated read

`GET /companies/{id}/360` serves the whole page. All below is one composite read, built inside a single
`database.WithWorkspaceTx`, and the response holds the `as_of` stamp of that read. The isolation level is
Read Committed (the posture of the platform). So a commit at the same time can land between two sections;
the stamp makes that visible. No section opens a second transaction. That is why every module store the
assembly calls offers a form of its read that takes the transaction.

**Authorization is per section.** Reading the company is required, and its refusal is the refusal of the
whole read (403/404 as usual). Every other section needs its own object grant. A section refused with
`apperrors.ErrPermissionDenied` is **left out of the payload**, and named in `sections_omitted`. It is
never sent as an empty array, so the UI can show "hidden from you" in place of an empty list. Empty
arrays would look like an account with no contacts, no deals and no history, and the rep would take them
as true.

The section names are spelled once (`company360/assemble.go`). They are both the contract's
`sections_omitted` enum and the keys the assembly works with, so a rename cannot leave the two sides
disagreeing:

`contacts` · `strength` · `deals` · `projects` · `activities` · `last_touch` ·
`state_strip` · `health` · `next_steps` · `next_meeting` · `tags` ·
`list_memberships` · `pending_approvals` · `since_last_visit` · `suggestions`

They run in a fixed order, so two reads of the same account give the same `sections_omitted` list. Any
error that is not a permission refusal fails the whole read. A section that failed for a real reason must
never be reported as one the caller may not see.

Two more rules keep the read correct by the way it is built:

- **A list inside the read is a summary**, not a surface to page through, and they do not share one shape.
  - The *paged* summaries (contacts, deals, activities, pending approvals, next steps) hold at most 25
    rows with `page.has_more`, and `page.next_cursor` is always null.
  - Page two comes from the endpoint that owns that list (`GET /activities`, `GET /deals`,
    `GET /relationships`, `GET /approvals`), each with its own cursor vocabulary.
  - `tags` and `list_memberships` are **plain arrays** with no page metadata at all. Suggestions report what
    they left out through `suggestions_dropped`, not a page object.
- **Every section keeps to the caller's scope**, with the same `platform/auth` predicate that each
  module list uses. So a section can never see more than the endpoint it covers. Where a section reports
  linked `ids`, those `ids` hold *their own* scope. A task reached through a visible contact must not hand
  back the id of a deal the caller may not read.

## The work in flight, and the account brief behind it

The lead card of the overview is **the account's work in flight**
(`frontend/src/screens/companywork.tsx`): one line per open deal, and one per live project. Each line
holds at most one reason it needs a look: an overdue task, or a promise they made to us that is still
open.

The server adds the reasons (`compose/company360/workattention.go`) in three queries over sets. They are
rendered through an `i18n` template over typed fields. No model writes anything on the card, so each line
can be checked against the record it links to. When nothing is in flight, the growth fit panel takes the
slot.

A written brief does not hold that place. On an account with more than one item of work under way, a
brief mixes them. Mail about one project becomes a sentence about another. A figure read out of the
mix has no place where it can be checked. A deal and a project are two stories. The card shows them as two groups
under their own subheads, never mixed.

`GET /companies/{id}/brief` (and `POST` for a rewrite a user asks for) still serves a brief, and is
**deprecated**: no screen renders it. It stays because `POST …/ask` is served from the same handlers and
the same assembly. It lives in `internal/compose/companybrief`, and owns one table, `company_brief`.

**Built as the caller.** The input of the brief comes from running the 360 itself, as the principal that
asked, inside the normal gates. The `Assembler` seam is injected, not imported. So the package uses one
seam, and does not work out the gated reads again itself.

So a brief can only describe records that caller could open themselves. `sections_omitted` goes
into the input, so the writer is told to stay silent about those subjects, and not to make things up around the
gap.

**Why not one shared brief per account?** It has no correct version. A shared brief written from what
all users can see together would leak scoped deals and activities to a reader with fewer rights. One
written from what all of them share would drop to the lowest common scope. It would tell the account
owner *less* than the page already shows them. So the cache is per viewer, keyed
`(workspace_id, user_id, company_id)`.

**Cached on the inputs, not on the record.** The key is a SHA-256 over the prompt version, the routing
version, and the assembled input encoded as JSON (`companybrief/input.go`). Facts, deals, activities and
grants all move without touching the `company` row. So a key taken from that row's version would serve a
brief that describes a pipeline the account no longer has, with no end. Putting the routing version in
means that pointing the model lane at a new model rewrites the briefs. It does not leave text that names
a model as the writer when that model no longer writes it.

A cached brief whose fingerprint no longer matches is **written again before the request answers**. So a
brief that the client gets is current. Serving the stale one, and refreshing behind the request, would be faster.
But it would give up that promise, and need a rewrite that lasts past the request. The code does not do
that, and nothing in the contract claims it.

**It steps down; it does not fail.** Take a case with no model lane set up, the workspace's AI
budget spent, or a reply the validator refuses. Then the brief falls back to a fixed, structured summary
over the same inputs (`companybrief/deterministic.go`).

- It covers identity, pipeline, each stalled deal on its own line, the last touch, open tasks. Then it
  covers what the company *is*, from its picked profile fields.
- Every fixed sentence cites the record it is from, the same way the model path does. So the card
  renders and acts the same, no matter which path wrote it.
- `generated_by` names which one it was, because a reader who decides how much to trust a sentence needs
  to know.

The prompt treats every activity subject and body as **untrusted quoted data**, behind a
nonce fence. That data is quoted between delimiter strings that are made new each time. So text inside it cannot pass as
part of the prompt. The site read prompts use the same rule. This text is from outside the workspace, and
must never be read as an order. The model runtime behind it is in [ai-runtime.md](ai-runtime.md).

Both the brief and Ask are **human only** (`x-agent-access: human-only`, `security: [{ cookieAuth: [] }]`,
and `auth.RequireHuman` at the service). A brief is a reading help for a human. An agent that reads records
through a passport has the records themselves.

## Ask

`POST /companies/{id}/ask` answers **one of three prepared questions** about the account: `whats_open`,
`meeting_prep`, `whats_changed`. The question is *chosen, not typed*, by design. Each prepared question
names the part of the account its answer may be written from. That lets every sentence hold a citation
the reader can open. A free text box would need retrieval that can prove what it did not find. And a box
that answered from a part, without saying so, would read like one that searched everything.

An unknown question is a 422, never a default. Answering a different question than the one asked looks
the same as answering the asked one badly.

Ask uses the parts of the brief again: the same input per viewer, the same nonce fence, the same
grounding filter, the same fixed floor. The three answers differ by one string each that says what to do. The
shared system prompt is strict about grounding:

- state only what the summary states;
- never state why, how anyone feels, what anyone plans, or a next step it does not hold;
- cite the `ids` the summary gave, in `evidence`, and never in a sentence's text;
- if the summary does not answer the question, return an empty array, not a sentence that goes around
  it;
- say nothing at all about anything named in `sections_omitted`.

Nothing is cached: a question is asked and read once. Ask does not get the company profile, because
those are approved statements, and none of the three questions is about them.

## Suggestions

The `suggestions` section is what the account looks like it needs, worked out from its own records.
**There is no model in this path.** The rules come from the contract enum, and are not spelled again:

| Kind | The rule |
|---|---|
| `no_reply` | an outbound message on a thread nobody answered (7 day window) |
| `stalled_deal` | an open deal with no activity past the 60 day stall window |
| `no_next_step` | an active account with no open task on it |
| `lifecycle_conflict` | a standing `contract_ended` signal, while the record still reads as a live customer or an open chance to sell |

Every rule is a check a rep could make themselves. **Each suggestion holds the rule in the words they
read** (`reason`: "the rule that fired, in the words the rep reads. Never a score.").

- It also holds the `evidence` records it fired on. Where the server can name one, it holds an `action`
  that opens a governed surface filled in from that evidence.
- `draft_reply` opens the composer on the message with no answer, and `open_deal` opens the stalled
  deal. `add_task` writes the step the server prepared, through the same `POST /tasks` that the task
  form uses.
- A rule that cannot name an action holds `null`, and the card gives advice with no button. A control
  that does nothing makes the reader stop pressing them.
- The page's handler covers every action kind. So a kind it has no surface for fails the build, and
  does not lose the click.

**A citation is a receipt, not a label.** Beside the record it points at, each evidence item holds what
the rule read from it:

- `quote`: the record's own words (a message's first line, the signal's sentence), as written, and never
  a rewrite in other words;
- `at`: the time it is dated (when the message was sent, when the deal was last worked);
- `origin`: where the words came from ("Email you sent", "Open deal, last worked").

The chip is labeled with the record's name, and opens that record when the pointer rests on it. The
words follow the audience test of the activity, and the row does not. A message whose content this reader
may not see still counts as the newest exchange. Its chip holds the date and the channel, with no quote.

None of it reaches the fingerprint, so a subject edited after the fact cannot bring back a dismissal. The
vocabulary lives in `suggestioncites.go`. A model could word these more kindly, but could not make them
something a rep can check. Being able to check lets a rep disagree with the *reason*, not with a verdict they cannot look
into.

Nothing is staged, and nothing is sent. Each rule runs under the same row scope predicates as the section
it is about, and only when the caller holds that section's grant. A suggestion can only point at records
they can open. A missing grant gives silence, not advice made up from the gap.

The card offers at most three, and the rest are reported in `suggestions_dropped`. A silent cap reads as
"that is everything". The cap is applied on the server, so the dropped count and the rows shown always
describe the same list.

**A dismissal is per user.** `POST /companies/{id}/suggestions/dismiss` takes the suggestion's
`fingerprint`: a hash over the kind, the subject and the records it fired on, not over the kind alone. So
advice stays gone *while the case holds*, and **comes back by itself when the evidence changes**, because
the case is then a new one.

A row is written only for a fingerprint the rules produce now for this account and this caller. That
bounds the table at one row per suggestion a human clicked. The two plain other choices both fail.
Accepting any fingerprint in a valid form makes the endpoint a write sink for any signed in caller.
Capping the stored count deletes the earliest choices without telling anyone, so a rep working through a
long list sees dismissed advice come back.

## What needs you, and the one answer it gives

The needs list has three sources: the moment leads it, the suggestions follow, and the moves made by hand
come last. The reader takes the top of it as the verdict. The moment fires on **owed promises only**; all
else comes through the suggestions, or the scan's findings. Two rules stop the list disagreeing with
itself, each held where it sees what the other cannot:

- **The quiet card claims only what it read.** It says *"Nothing is owed to this account"*, never
  "nothing needs you today". The second would be a claim about work it did not look at, in the payload
  that holds the rows that say the opposite. Every client reads it, the tool surface too, so the bound sits
  on the server.
- **A quiet card is no row beside others** (`momentIsARow`). Only the client can apply this, because the
  scan's findings come on their own read. The row goes only where the list holds something, and what it
  holds *is* the answer.

The 360 is also a **live read**: every 60 seconds while the tab has focus. A page read once when it
opens goes stale without any sign, and a stale "what needs you" looks like a current one. It is a poll on
a fixed schedule, not a stream; the contract serves no push. A read is live for the key it reads under (`isRecordRead`),
not because its screen asked for it. So the contact, project and deal records are live too, with the
deal's open requests and tasks.

## The account scan

The rules say what the *records* show. The account scan is what the *exchanges* say, read by a model. It
is the one model call the page makes on its own account, so the design is mostly about how it is bounded.

**What it reads.** The scan's input is the reader's own composite read: the brief's projection of it, with
the same deals, tasks and subjects. So the two surfaces cannot disagree about what a record is called.

- It adds the last 20 exchanges with their own words. Each body is cut at 1,200 characters, and the
  cut is reported.
- The words are read under the activity **content** gate, not the `discover` gate. A message the reader may
  know exists, but may not read, is not in the input. So the model cannot quote it.
- A reader with no activity grant is refused, and is not shown a quiet account.

**What it may say.** The *read* kinds sit beside the rule kinds:

- `commitment_unmet`: we said we would do something, and nothing says it happened;
- `question_unanswered`: they asked, and no later message of ours answers;
- `risk_raised` and `need_raised`.

How a finding is held to the input:

- A finding names one message by an id the model was handed, and quotes it as written. The reply schema
  is built per call, with that reader's message `ids` as the citation enum. So a made up id fails the
  provider's own check.
- The parser then holds every finding to the input. It checks the kind, the id, and that the quote is a
  part of that message's text, once white space is made the same. That check is `claims.Quoted`, shared
  with the field `extract` and the corpus ask. It also checks that no id is in the text.
- A finding that fails is dropped whole, never shown with its citation stripped. What is left is the same
  suggestion shape the rules produce, `written_by: model`. It holds the message's subject, date, channel
  and the quote as its receipt.
- Its fingerprint comes from the same helper the rules use (`company360.SuggestionFingerprint`).

**One list, one dismissal.** `GET …/scan` answers with the merged advice. The rules run live through
`company360.Service.UndismissedAdvice`, and the model's stored findings are filtered through
`KeepUndismissed`. The two lists drop copies by fingerprint, and are capped at five, with the cap
reported. Dismissing a finding goes through the same endpoint as dismissing a rule's row. The dismissal on
the 360 asks the scan, through `RecogniseScanFindings`, whether a fingerprint its rules do not raise is
one the reader's stored scan does.

**Only when asked, never a sweep.** Opening the page calls `POST …/scan` once. The server takes a fingerprint of
the input: the floor and prompt versions, the routing version read live, the language, and the encoded
input.

- It answers with the stored findings when the fingerprint matches.
- Say the account moved, but the reader's last read is newer than the **one hour floor for a new
  scan**. Then it answers with the same findings marked `stale`. If not, it queues a read.
- A read in flight is returned as it stands, and not started twice. `force` skips the floor and the
  fingerprint, never the check for a read in flight.
- A reader who never opens an account never pays for it. And a busy inbox does not read the account
  again on every message.

**The row is the carrier.** `company_scan` holds one row per (reader, account). It holds the read in
flight: the `status` in the vocabulary of the AI activity rail, and the attempt. It also holds the
timestamp fields that carry the lease, and `next_attempt_at` for a budget deferral. It also holds the last findings that settled. Those
are kept while a new read runs, so the page is never blank when the account is busiest.

The api role writes the row and the `account_scan` job in one transaction. The worker role claims the
row, and binds again the **reader's own principal**: their grants, teams and seat through
`identity.EffectiveAuthority`. It never uses a system principal with their name on it, because a system
principal reads every audience away. It uses the row id as the correlation id, reads, and settles.

Every change of state reports itself on the rail, with the account's name as the subject. So a reader who
opened three accounts and moved on finds out which is ready. A budget deferral parks the row and puts the
job to sleep. No lane, or a reply that the grounding refused whole, settles `degraded` with a reason the
reader can read, and the rules' rows stand alone.

An account with no exchange the reader may read settles `done`, not `degraded`. The read covered all it
was going to. `read` reporting zero exchanges, beside `generated_by` saying the rules wrote the advice, is
the whole of what happened. `degraded` reaches the fault arm of the rail, which holds the orb amber until
someone confirms it. Marking an empty account `degraded` would raise a warning on every account nobody has
written to yet. That is every account on the day it is added.

**The page.** While the read runs, the needs list keeps the rules' rows. It draws the `AiPending` row
above them (the moving indigo tile, and lines of the answer still being drawn). It polls every three
seconds until the row settles.

Then the merged list replaces the own rows of the 360. The foot says who wrote them, and how many
exchanges and deals were read. It says that the account has moved since (where it has), or why the model
did not write them.

## The state strip and health

A single relationship score over the account's contacts would let one contact who writes often speak for
the whole account. And nobody could say what scale it was on. So the header leads with three values
instead.

**The state strip** is the three values the overview leads with.

- The *account* half (lifecycle, relationship types) needs no grant beyond the company the caller already
  read. Each other value needs its own grant.
- *Engagement* (last inbound, last outbound, a state worked out from them) needs the timeline grant.
  *Commercial* (open count, stalled count) needs the deal grant. The *signal* slot holds the worst thing
  standing open.
- Each is **null when refused, never zero**. "No open deals" and "you may not see the deals" are
  different facts, and only one of them is about the account.
- A null on the signal slot covers both "nothing is wrong" and "you may not read signals". A strip that
  told someone all is well, when they cannot look, would answer a question it has no right to answer.

Last inbound and last outbound are two timestamp fields, not one "last touch", because **which side wrote last
is the question**. Take an account we mailed two weeks back with no reply, and one that wrote to us this
morning. They have the same last touch date, and mean opposite things.

Both walk the same three links the timeline walks, so the header can never disagree with the list under
it. And both hold the caller's activity row scope, so a rep sees the last message *they* may read.

**Health** breaks the relationship into parts a reader can act on:

- days since the last inbound;
- active contacts (contacts who have been in touch; a roster of ten who never replied is not ten ways
  in);
- the reply balance over the 90 day window;
- `single_threaded`: one contact who holds the whole relationship is the one shape a rep can fix before
  it costs them the account. So it is named, not scored;
- open promises.

The same rule applies: every part is null when it cannot be worked out, because zero is a claim about the
account.

The signals *card* is separate. It reads `GET /signals` itself with `status=open`. So it owns its own
loading, `unavailable` and empty states, and does not take those of the 360.

## The visit baseline

`since_last_visit` is what changed on the account since **this caller** last confirmed seeing it: new
activities, deal stage moves, pending proposals. Each is null when the grant for it is missing. So "not
counted" stays apart from "counted as zero".

**Only a call for that moves the baseline**, `POST /companies/{id}/view-ack`. A GET that moved it
as a side effect would erase the answer the caller opened the page to read. It would also make a fetch
done ahead of time look like a visit. The upsert only moves forward (`GREATEST(stored, now)`). So the late
ack of a slow tab can never move a newer one back. Two tabs on the same account end on the later visit,
and do not fight to move the baseline back.

It is **human only**, and that gate is required, not a second layer of guard. An agent principal holds
the granting human's id as its `UserID` (that is how row scope works for passports). So "resolve the
acting user" would write a baseline that marks an account as *seen* by a human who never opened it. That
would use up their unread marker for them.

**The client waits before it sends.** `useAcknowledgeCompanyView` waits `VIEW_ACK_DWELL_MS` (5 seconds)
with the account open before firing, and leaving cancels the timer. Opening a record and going right back
out is not reading it. An ack from that would mark unread activity as seen. Only an *assembled* 360 counts
as a visit.

Success does **not** clear the cached 360 query. The "new since your last visit" line describes the visit
under way. Fetching it again under the reader would erase what they opened the page to see. When the client cannot tell,
the baseline stays put. Showing an item twice is a smaller wrong than hiding one, so a failed ack is
not shown as an error.

## The logo

`Company.logo_url` points at `GET /companies/{id}/logo`. The mark is resolved during a deep read, from the
page that read already fetched (its `og:image` and its declared icons). So a picture for every company costs
no logo API from a third party, and no new egress beyond the file itself.

- Candidates are tried in a fixed order (at most 8). Each must be between `32px` and `300px` on the long
  edge. One so much wider than it is tall that it says "banner", not "mark", is rejected.
- The chain picks first the square icons a site declares, because this mark sits inside a round avatar
  on every record card.

All that is stored is **encoded again once, at store time**. The endpoint always answers `image/png`, no
matter what type the source file was. So no markup from a third party is ever served from this origin. The
response is served `nosniff`, with `Content-Security-Policy: default-src 'none'; sandbox` and a short
private cache.

A logo a human uploaded is never replaced by one a machine found. The write takes a row lock and checks
`field_provenance` under it. So the rule of which one wins holds on a run at the same time, as well as on
the quiet ones.

**This needs object storage.** `MARGINCE_BLOBSTORE_ENDPOINT` and the values that go with it (see
[../reference/configuration.md](../reference/configuration.md)) turn it on. With no blob store set up, the
resolve lane returns before it fetches anything. So no `logo_object_key` is ever written, and the endpoint
answers 404 for every company.

A deployment that *had* a store, and then has none, answers `501 not_implemented` for records that still name an
object. All three answers render the same thing. A 404 also covers "hidden from the caller" and "does not
exist", and telling them apart would leak which companies exist.

The floor is the **fixed monogram**. `Avatar` takes the initials from the name. When tinted, it picks one
of six tone pairs from a stable hash over the code points of the name. So the same record shows the same
color everywhere, without storing one.

The monogram renders *under* the image. So it is what shows while the logo loads, what is left if the
image fails, and what a company with no resolved logo has. A company is never a broken image or an empty
slot.

### The installation's second mark

The record above shows one picture. The installation's own company shows two, because the sidebar draws
it at two sizes. The wide lockup fits an open panel. A **square icon** serves the `56px` rail, where a
wordmark scaled into a `32px` box is a row of lines nobody can read.

The icon is a second pair of columns on the same row (`logo_icon_object_key`, `logo_icon_origin`). It is
read back as `CompanyProfile.logo_icon_url`, and served from `GET /companies/{id}/logo/icon` under the
rules above. It is encoded again the same way, with the same headers. It gives the same 404 for missing,
hidden and not existing.

Two writers reach it: the `cold-start` website read, and `uploadCompanyLogoIcon`. A user's upload wins
over the read, under the same provenance check the writers of the wide mark take. Every company but the
anchor answers 404 for it.

**The cold-start read resolves both marks.** The onboarding read is the one read whose company is drawn
at two sizes. So it runs two chain passes over the seed page it already fetched.

- The **lockup** comes from what the page itself calls its logo. First it tries the `schema.org` `logo`
  its JSON-LD declares. Then it tries the `<img>` elements it labels as one (in the `alt` text, the class,
  the id or the file name).
  - This search reads the page body, which the icon search does not. A lockup lives in no other place, and
    the label is the evidence.
  - The reader who reviews the dossier sees the mark before any record shows it. It is stored with its
    shape kept, at the edge of the upload path, so both writers of the wide slot store the same shape.
- The **badge** comes from the chain above (the `apple-touch-icon`, the favicons, `/favicon.ico`, the
  `og:image` last). Only a square result is stored as one.

The badge chain runs first. The lane runs under one deadline, and a slow lockup fetch must never cost the
company its square mark.

When no lockup resolves, the badge fills the wide slot, and the icon slot stays empty. When the lockup is
itself square, or the best mark of the chain is not, no badge is stored either. The rail, when closed, falls
back to the wide mark on its own, so a second copy would be bytes stored for nothing.

Both marks wait on the dossier (`site_read.logo_object_key`, `site_read.logo_icon_object_key`). The
confirm binds each to its slot on the record, slot by slot, under the rule that a human's choice wins.
Enrich reads of every other company keep resolving the one mark, with a square picked first. A wordmark boxed
into a record card's round avatar would be the row of lines nobody can read that the badge exists to
avoid. `worker siteread` reports both slots, and every candidate each one tried.

The two slots are chosen and cleared separately in settings. The rail, when closed, falls back to the
wide mark when there is no icon.

## The connections card

`GET /companies/{id}/graph` is a **second** read serving the same page. It holds the records one hop out from
the account, as a node and edge set the browser draws. It is separate from the 360. A client that wants the
profile does not always want the graph, and its unit of authorization is a **node group**, not a section.

The posture is the same: one transaction, one point in time, grants per group, and a cap that reports what it
left out. The groups are `contacts`, `deals`, `intro_path` and `our_side`, named in `groups_omitted` when
refused.

**One hop means one edge from the account.** A contact's other companies, a deal's other accounts and a
partner's own partners are not walked. A second hop is a different read with a different cost. And a card
that could go one hop or two could not state a cap.

The display caps are what fits a picture a rep reads at a look: 15 contacts, 10 deals, 10 related
companies, 10 colleagues. A scan bound of 500 applies to the one group whose display order the database
cannot know. Contacts are ordered by a relationship strength that is worked out after the read.
Stakeholder contacts have no cap of their own; the deals already selected bound them.

`dropped_count` keeps the read in line with the account, not with the caps. It is counted over **all the members** of each
group, not over the rows in hand. A cut graph that reports no count reads as all the records one hop
out. A count taken from a bounded read would report too few.

So the caps bound the rows returned, and the work per contact done on them (the part that grows fast).
And a full count of the rest costs one index range scan per account. It stays true past the
scan bound of 500 contacts too.

Graph details beyond the card (the strength model, the warm intro resolver, our side edges) are in
[relationship-graph.md](relationship-graph.md).

## View state is not record fact

Three tables behind this page live in a `compose` subpackage each, and all three are written **without an audit
row or outbox event**. This is the ruling for saved views, gated by `backend/gates/tableownership_test.go`:

| Table | Owner | What it is |
|---|---|---|
| `user_record_view` | `internal/compose/company360` | the visit baseline per user |
| `suggestion_dismissal` | `internal/compose/company360` | the rep's "not this, not now" |
| `company_brief` | `internal/compose/companybrief` | the brief cache per user |

This is the exception to the shape in [write-backbone.md](write-backbone.md) (domain row + `audit_log` +
`event_outbox`), and it is small by the way it is built.

- Each of the three is written on a *view* action (a visit, a click, a rewrite). Only its own user can read
  it, and no reader of the bus can act on it. In the brief's case, it is content that can be made again
  at any time.
- None of them is a fact about the record. An audit log of who looked at what, sent onto the bus, would
  be watching users, not provenance.
- The ruling is recorded inline against each entry, so the gate needs nothing else on a clean checkout.

Both of those `compose` subpackage directories follow the rules of the compose layer in all else. They link modules (company,
contact, relationship, deal, activity, tag, list, approval, signal), and own no business record for the
long run. See [composition-layer.md](composition-layer.md).

## Where the code lives

| | |
|---|---|
| The composite read + its section registry | `backend/internal/compose/company360/assemble.go` |
| Section vocabulary, caps, row scope predicates, next steps | `backend/internal/compose/company360/sections.go` |
| Contacts / deals / tags + lists / signal facts | `backend/internal/compose/company360/{contacts,deals,collections,signalfacts}.go` |
| The state strip, last touch, health | `backend/internal/compose/company360/accountstate.go` |
| Suggestion rules and their reads | `backend/internal/compose/company360/{suggestions,suggestionreads}.go` |
| The dismissal store (`suggestion_dismissal`) | `backend/internal/compose/company360/dismissal.go` |
| The advice seam the scan merges with, and the shared fingerprint | `backend/internal/compose/company360/advice.go` |
| The account scan: input, words, prompt, grounding | `backend/internal/compose/companyscan/{input,words,write}.go` |
| The account scan: row, rail carrier, the rule for `ensure`, merge | `backend/internal/compose/companyscan/{store,service}.go` |
| The account scan's job, and its wiring into both roles | `backend/internal/compose/jobs_accountscan.go` |
| The visit baseline (`user_record_view`) | `backend/internal/compose/company360/viewbaseline.go` |
| The account card, the row every record page draws it as, and the moment vocabulary they read it with | `backend/internal/compose/company360/moment.go`, `frontend/src/screens/record360/today.tsx`, `frontend/src/screens/record360/moment.ts` |
| The live record poll, and which reads it knows | `frontend/src/app/queryclient.ts`, `frontend/src/screens/activitykeys.ts` |
| The connections graph | `backend/internal/compose/company360/{graph,graphreads,graphplace,graphourside}.go` |
| HTTP transport | `backend/internal/compose/company360/handlers.go` |
| The brief: cache, input, fingerprint | `backend/internal/compose/companybrief/{service,input}.go` |
| The brief: model path and its validator | `backend/internal/compose/companybrief/write.go` |
| The fixed floor | `backend/internal/compose/companybrief/deterministic.go` |
| The prepared questions | `backend/internal/compose/companybrief/ask.go` |
| Logo resolve (candidates, normalize, store; the `cold-start` lockup and slot decision) | `backend/internal/compose/{sitelogo,sitelogocandidates,sitelockup}.go` |
| Logo row, which provenance wins, `LogoURL` | `backend/internal/modules/contacts/companylogo.go` |
| Logo streaming handler | `backend/internal/modules/contacts/handlers_company.go` |
| Contract | `backend/api/crm.yaml`: `/companies/{id}/{360,graph,brief,ask,view-ack,suggestions/dismiss,scan,logo}` |
| The ruling on who owns which table | `backend/gates/tableownership_test.go` |
| The screen | `frontend/src/screens/companies.tsx` (`CompanyScreen`) |
| The scan on the page: `ensure` on open, poll, the pending row | `frontend/src/screens/accountscan.tsx`, `companytoday.tsx` |
| Data layer + cards in the right rail | `frontend/src/screens/company360.tsx`, `company360.css` |
| The connections view to compare | `frontend/src/screens/coverageexplorer.tsx` on the Contacts tab, with `companygraph.ts` as its read |
| Header actions (new deal, tag, list) | `frontend/src/screens/companyactions.tsx` |

## Where to go next

- [relationship-graph.md](relationship-graph.md): the graph under the connections card.
- [company-context.md](company-context.md): the *installation's* own company, a different subject with a name much like this one.
- [authorization.md](authorization.md): the grants and row scopes every section asks for.
- [composition-layer.md](composition-layer.md): why this lives in compose.
- [ai-runtime.md](ai-runtime.md): the model lane behind the brief and Ask.
- [../reference/configuration.md](../reference/configuration.md): object storage for the logo.
