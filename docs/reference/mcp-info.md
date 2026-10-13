# The served MCP surface

<!-- Generated together with mcp-info.json; do not edit by hand. -->

Generated from the served MCP surface by `go test ./internal/compose/ -run TestPublishedMCPSurface -update-mcp-info`; do not edit by hand. It shows every tool and resource across all scopes. tools/list and resources/list are filtered per caller, so a passport holding fewer scopes is served less. Extension tools are left out. The surface is captured as a client that supports MCP Apps sees it, so a tool bound to a view carries `_meta.ui.resourceUri`. A client that declines the UI extension is served no such member, and a client on the older handshake protocol, which cannot declare the extension, is served views. The `ui://` view descriptors are included; a deployment publishes each only after its boot has fetched and admitted that document.

`mcp-info.json` beside this page is the same surface byte for byte, as a client
receives it, with every input and output schema. This page is rendered from that file and
carries each tool's description. What a tool may do and whether a human must approve it is in
[agent-tools.md](agent-tools.md).

## Totals

| | |
|---|---:|
| Tools | 56 |
| Resources | 9 |
| Tool catalog | 173.0 KB |
| Resource catalog | 3.5 KB |
| Approx. wire tokens | 45179 |
| Largest tool | `prep_for_meeting` (9.0 KB) |
| Scopes rendered | `read`, `draft`, `write`, `send`, `enrich` |

Those are the bytes on the wire: they carry each tool's output schema and the
governance clause the transport appends. The listing a run re-sends every step is
smaller (name, description and input schema only) and is held against its own
budget in `agenttooldescriptions_test.go`. The Prompt tokens column below is what each
tool adds to that listing, and what it costs each scheduled agent, agent by agent, is
[agent-tool-budget.md](agent-tool-budget.md).

### What the tool catalog is made of

| Part | Bytes | Share | In a run's prompt? |
|---|---:|---:|---|
| Output schemas | 58.6 KB | 33% | **No**: a result's shape, never listed to a model |
| Descriptions (incl. governance clause) | 48.4 KB | 27% | Yes, every step |
| Input schemas | 52.6 KB | 30% | Yes, every step |
| _Names, annotations, punctuation_ | 13.4 KB | 7% | Partly |
| **Description + input schema** | **101.0 KB** | **58%** | **the recurring cost** |

Output schemas are the largest part of the total and are never sent to a model;
descriptions and input schemas are what each step pays for.

## Index

### Resources (9)

- [`margince://capabilities`](#capabilities): What this installation can do
- [`margince://schema/query`](#query_vocabulary): Workspace query vocabulary
- [`margince://schema/record-fields`](#record_fields): Record write vocabulary
- [`margince://schema/reports`](#report_vocabulary): Report plan vocabulary
- [`margince://schema/report-blocks`](#report_blocks): Report block grammar
- [`margince://schema/analytics`](#analytics-schema): Analytics query vocabulary
- [`ui://margince/create-followups.html`](#create_followups_view): Next steps for this record
- [`ui://margince/approval.html`](#approval_view): Waiting for a decision
- [`ui://margince/field-conflict.html`](#field_conflict_view): Edited by hand

### Tools (56)

| Tool | What it is for | Read-only | View | Prompt tokens |
|---|---|:-:|---|---:|
| [`advance_project_phase`](#advance_project_phase) | Move a project to a phase |  |  | 416 |
| [`annotate_brief`](#annotate_brief) | Write findings onto the morning brief |  |  | 417 |
| [`archive_record`](#archive_record) | Archive a record |  |  | 292 |
| [`at_risk_relationships`](#at_risk_relationships) | Relationships going cold | yes |  | 212 |
| [`attach_document`](#attach_document) | Attach a file to a record |  |  | 399 |
| [`book_meeting`](#book_meeting) | Book a meeting |  |  | 344 |
| [`bulk_update_records`](#bulk_update_records) | Change many records at once |  |  | 809 |
| [`catch_me_up_on`](#catch_me_up_on) | Catch me up on a record | yes |  | 348 |
| [`change_lead_state`](#change_lead_state) | Change a lead's state |  |  | 494 |
| [`change_lists`](#change_lists) | Make and change lists |  |  | 592 |
| [`change_tags`](#change_tags) | Change tags |  |  | 492 |
| [`check_availability`](#check_availability) | Check calendar availability | yes |  | 342 |
| [`commit_import`](#commit_import) | Commit an import |  |  | 279 |
| [`compose_analytics_report`](#compose_analytics_report) | Compose an analytics report | yes |  | 786 |
| [`create_record`](#create_record) | Create a record |  | [`ui://margince/create-followups.html`](#create_followups_view) | 678 |
| [`create_task`](#create_task) | Create a task |  |  | 221 |
| [`decide_approvals`](#decide_approvals) | Approve or reject staged actions |  |  | 429 |
| [`describe_vocabulary`](#describe_vocabulary) | Describe what a tool accepts | yes |  | 346 |
| [`draft_outbound`](#draft_outbound) | Draft an email or follow-ups |  |  | 517 |
| [`enrich`](#enrich) | Enrich a company from its website |  |  | 396 |
| [`invite_meeting`](#invite_meeting) | Send a calendar invitation |  |  | 264 |
| [`list_approvals`](#list_approvals) | List what is waiting for a decision | yes |  | 268 |
| [`list_documents`](#list_documents) | List a record's documents | yes |  | 180 |
| [`list_records`](#list_records) | List records | yes |  | 821 |
| [`log_activity`](#log_activity) | Log an activity |  |  | 791 |
| [`merge_tags`](#merge_tags) | Fold one tag into another |  |  | 198 |
| [`prep_for_meeting`](#prep_for_meeting) | Prepare for a meeting | yes |  | 394 |
| [`preview_import`](#preview_import) | Preview an import |  |  | 986 |
| [`progress_deal`](#progress_deal) | Progress a deal with a note |  |  | 522 |
| [`query_workspace`](#query_workspace) | Query the workspace | yes |  | 530 |
| [`read_approval`](#read_approval) | Read one staged action in full | yes | [`ui://margince/approval.html`](#approval_view) | 154 |
| [`read_brief`](#read_brief) | Read the morning brief | yes |  | 205 |
| [`read_directory`](#read_directory) | Read the workspace's colleagues, pipelines and channels | yes |  | 361 |
| [`read_forecast`](#read_forecast) | Read the forecast and how it moved | yes |  | 677 |
| [`read_import`](#read_import) | Read an import run or the import format | yes |  | 185 |
| [`read_input_checks`](#read_input_checks) | Can the forecast's inputs be trusted | yes |  | 346 |
| [`read_lists`](#read_lists) | Find and read lists | yes |  | 745 |
| [`read_project`](#read_project) | Read a project or its handover | yes |  | 292 |
| [`read_record`](#read_record) | Read a record | yes |  | 292 |
| [`read_relationships`](#read_relationships) | Read who we know, and how well | yes |  | 359 |
| [`read_reporting`](#read_reporting) | Read sales reporting | yes |  | 544 |
| [`read_tags`](#read_tags) | Read tags | yes |  | 322 |
| [`relink_activities`](#relink_activities) | Re-associate a set of activities to a record |  |  | 260 |
| [`relink_activity`](#relink_activity) | Re-associate an activity to a record |  |  | 303 |
| [`resolve_duplicates`](#resolve_duplicates) | Merge two records or dismiss a flagged pair |  |  | 393 |
| [`resolve_entities`](#resolve_entities) | Resolve contacts and companies | yes |  | 494 |
| [`review_commitments`](#review_commitments) | Review open commitments | yes |  | 401 |
| [`run_analytics_query`](#run_analytics_query) | Run an analytics query | yes |  | 508 |
| [`run_report`](#run_report) | Run a report | yes |  | 1024 |
| [`search_context`](#search_context) | Search for relevant material | yes |  | 344 |
| [`search_records`](#search_records) | Search records | yes |  | 456 |
| [`search_report_evidence`](#search_report_evidence) | Search the evidence behind a saved run | yes |  | 335 |
| [`send_outbound`](#send_outbound) | Send a message |  |  | 1014 |
| [`update_record`](#update_record) | Update a record |  | [`ui://margince/field-conflict.html`](#field_conflict_view) | 581 |
| [`whats_slipping_this_week`](#whats_slipping_this_week) | What's slipping this week | yes |  | 273 |
| [`whoami`](#whoami) | Who this passport acts for | yes |  | 129 |

## Resources

A resource takes no arguments and changes nothing, so it carries no autonomy
tier. It is scope-filtered as a tool is, so a passport holding
fewer scopes is served fewer documents.

### capabilities

`margince://capabilities` · application/json

**What this installation can do**

```text
The verbs this passport may call, which of them execute directly and which stage for a human
decision, and the scopes it holds. Names and governance only — the input schemas live in
tools/list.
```

### query_vocabulary

`margince://schema/query` · application/json

**Workspace query vocabulary**

```text
Everything a query plan may say, for you: the record types you can ask about, the fields you can
name on each, the operators each field admits, and the single relationship hop a plan may take. A
plan naming anything outside it is refused rather than approximated.
```

### record_fields

`margince://schema/record-fields` · application/json

**Record write vocabulary**

```text
The fields create_record and update_record accept for each record_type: which are required, what
shape each takes, and the values the closed ones admit. The two tools name this document instead of
carrying it.
```

### report_vocabulary

`margince://schema/reports` · application/json

**Report plan vocabulary**

```text
What each prebuilt report accepts in a run_report plan: the names its group_by, filters and
aggregates admit, what it answers with no plan at all, and what a filter means when its name alone
does not say. run_report names this document instead of carrying it.
```

### report_blocks

`margince://schema/report-blocks` · application/json

**Report block grammar**

```text
The blocks a report may carry: each kind, whether it renders figures, words or both, and the
severities a callout may state. compose_analytics_report names this document instead of carrying it.
```

### analytics-schema

`margince://schema/analytics` · text/plain

**Analytics query vocabulary**

```text
The populations a run_analytics_query plan may name, each with its group_by dimensions and its
measures, derived for this seat. run_analytics_query names this document instead of carrying it.
```

### create_followups_view

`ui://margince/create-followups.html` · text/html;profile=mcp-app

**Next steps for this record**

```text
What a new record leaves to decide: a possible duplicate with the choice to merge it or keep it
apart, and a tag word offered for it.
```

<details><summary>Sandbox policy (<code>_meta.ui</code>)</summary>

```json
{
  "ui": {
    "csp": {
      "baseUriDomains": [],
      "connectDomains": [],
      "frameDomains": [],
      "resourceDomains": []
    },
    "prefersBorder": true
  }
}
```

</details>

### approval_view

`ui://margince/approval.html` · text/html;profile=mcp-app

**Waiting for a decision**

```text
A change an assistant proposed, what it would do, and the choice to approve or reject it.
```

<details><summary>Sandbox policy (<code>_meta.ui</code>)</summary>

```json
{
  "ui": {
    "csp": {
      "baseUriDomains": [],
      "connectDomains": [],
      "frameDomains": [],
      "resourceDomains": []
    },
    "prefersBorder": true
  }
}
```

</details>

### field_conflict_view

`ui://margince/field-conflict.html` · text/html;profile=mcp-app

**Edited by hand**

```text
Fields last edited by hand that an update would overwrite, with the value on the record beside the
one proposed and the choice to keep or replace it.
```

<details><summary>Sandbox policy (<code>_meta.ui</code>)</summary>

```json
{
  "ui": {
    "csp": {
      "baseUriDomains": [],
      "connectDomains": [],
      "frameDomains": [],
      "resourceDomains": []
    },
    "prefersBorder": true
  }
}
```

</details>

## Tools

### advance_project_phase

**Move a project to a phase**

```text
Move a project to another phase — initiative, pursuing, delivering, closed. The four names are
fixed but the order is not enforced: a project may go back a phase, and a closed one may be
reopened. Closing requires a reason, which is recorded on the phase history either way. Closing does
not complete, cancel or hide anything open on the project: its tasks and commitments stay open on
their own records. When the user has asked for the close, report what is still open and close — an
open commitment is something to tell them, not a reason to hold back a close they asked for. Use
progress_deal for a deal's pipeline stages; a project's phases are a different vocabulary on a
different record. Send if_version with the version you read. By default the phase moves when this
call answers. Where an installation has raised this verb to confirm first, the answer is a staged
approval and the phase has NOT moved — keep its id and do not report the project as advanced until
the retry that carries the approval has answered. (Runs immediately; requires passport scope
"write".)
```

### annotate_brief

**Write findings onto the morning brief**

```text
Write what you found onto the morning brief you just read: one sentence about the night as a whole,
and for each deal you looked at, why it is on the list, what changed, and the one next move you
would make. It writes onto that user's own brief for today and nothing else — it cannot be pointed
at another user, another day, or a deal that is not already in their queue, and it cannot change the
ranking. Every evidence id you cite must be one the brief already recorded for that item; citing
anything else refuses the whole write, so cite from what read_brief gave you rather than from
memory. Use log_activity to record something that happened on a deal, which belongs on the record
itself and outlives today's brief. Calling it again replaces what you wrote before, so a second pass
is a correction rather than an addition. (Runs immediately; requires passport scope "write".)
```

### archive_record

**Archive a record**

```text
Retire a record that should no longer be worked — a duplicate, a dead company, a project that
ended. Archiving hides the record from day-to-day work; it does not delete it and does not move
anything attached to it, so an archived duplicate still holds the activities and deals that were
logged against it. Use resolve_duplicates when a duplicate's history should end up on the record
that survives, and change_lead_state to disqualify a lead going nowhere — a lead's own transition
records the reason where archiving would not. By default the record is archived when this call
answers; where an installation has raised this verb to confirm first, the answer is a staged
approval and you must not report the record as archived until the retry that carries their approval
has answered. (Runs immediately; requires passport scope "write".)
```

### at_risk_relationships

**Relationships going cold**

```text
Answer "where are our relationships thin?": across the caller's OPEN deals, the ones resting on a
single contact, missing an engaged champion, or carried almost entirely by one colleague on our
side. It sweeps open deals — a deal already won or lost is not at risk and is left out — and it
takes no arguments, because the caller's own visibility already decides which deals these are. It is
about the shape of the relationships around a deal, not about the deal's own momentum. Use
whats_slipping_this_week when the question is about deals losing momentum, and read_relationships
with a deal_id when the question is about one deal rather than the whole book. Each finding names
its deal_id and the contacts it is about; those are what read_relationships takes next.
```

### attach_document

**Attach a file to a record**

```text
Put a file the user gave you on a company, contact, deal, lead or project, where it appears on the
record's Documents tab, or on one activity such as the meeting it came from — use it whenever the
user wants a file kept on a record, and never save a file's text as a note instead. Up to 6.2 MB per
file, or less where the workspace sets a smaller upload limit. Common document, image and email
formats are accepted; any other kind is refused, and the refusal names the accepted ones. Tell the
user about a refused file; never rename, convert or zip it to get it accepted. The file is stored,
not read. It is filed against a contract only when you name contract_id; read_project (view:
overview) lists its contracts with their ids. Use log_activity for what was said about the file,
linked to the same record. Keep attachment_id to name the file to the user. (Runs immediately;
requires passport scope "write".)
```

### book_meeting

**Book a meeting**

```text
Record a meeting against linked CRM records without sending an invitation. Reserves the recorded
interval locally. It does not create a calendar event or notify attendees. Needs at least one record
link. Use invite_meeting for an explicit provider-backed invitation, or log_activity for a past
meeting. Keep the recorded activity id. Do not describe a record-only booking as an invitation sent.
(Runs immediately; requires passport scope "send".)
```

### bulk_update_records

**Change many records at once**

```text
Hand up to 500 contacts, companies, deals or leads to one owner, archive them, add them to or take
them off a Shortlist, put a tag on or take it off, or file one task under each, in one change; or
mark up to 500 of the user's Worklist tasks done (record_type worklist_item, verb complete) — or
undo such a change. A Worklist commitment is not yours to mark done: it is skipped as
commitment_needs_the_user. Call mode preview first and show the user what it says: how many records
change, which are left alone and why, and the sample rows. Execute only after they agree. Each
record is changed only if it still has the version you sent and you may change it. To undo, pass the
batch_id to undo_preview, show the user the answer, and call undo after they agree; a change is
undone once, and records changed since are left alone. update_record and archive_record change one
record. Above 10 records, execute and undo need the confirm_token their preview answered; it is good
once. Keep batch_id from the answer: undo names the change by it. (Runs immediately; requires
passport scope "write".)
```

### catch_me_up_on

**Catch me up on a record**

```text
Answer "what has been going on with this?" for one contact, company, deal, lead, project or meeting:
the recent activity and related records in one picture, with the evidence each part rests on. Built
around ONE record you name; everything it reports carries a source, and what cannot be evidenced is
absent rather than inferred. prep_for_meeting when a meeting is about to happen, read_record for the
record's own stored fields, search_records when you do not yet know which record you mean. Each item
carries the record_type and record_id a follow-up call acts on. occurred_at is when an item
happened, in UTC — prefer it over a date the prose recalls, and convert before naming a day.
```

### change_lead_state

**Change a lead's state**

```text
Move a lead through its lifecycle. qualify fills what the lead's own data already implies — today
the company name, from its email domain — and names the qualification fields still empty. promote
turns a lead who has genuinely engaged into a contact, carrying their history across. disqualify
closes out a lead that is going nowhere. demote reverses a promotion that should not have happened.
qualify fills only empty fields and never overwrites or invents a value. promote needs a trigger
naming the engagement — a reply, a booked or held meeting, or a human's decision; cold outreach
nobody answered never promotes. disqualify keeps the record and its history. demote needs a reason
and is refused once the promoted contact owns a deal. Use enrich to learn about a company from its
website, and update_record to edit a lead's fields. The gaps qualify reports are what a human still
has to supply. A promotion answers with the new contact's id. Where an installation has raised an
action to confirm first, the answer is a staged approval: keep its id and do not report the change
until the retry carrying it has answered. (Runs immediately; requires passport scope "write".)
```

### change_lists

**Make and change lists**

```text
Make a Live List from a filter or a Shortlist of chosen records, change its name, purpose, filter,
sharing or steward, archive or restore it, and add or remove one Shortlist member with a note on
why. Only the steward of a list or a list admin may change it. A change must carry the version you
read; a list changed since is refused. An archived list is read-only. A Live List's members follow
its filter and cannot be added or removed by hand. Preview a filter with read_lists before saving
it. bulk_update_records adds or removes many records at once, with a confirmation. Keep the list id
and its new version from the answer. (Runs immediately; requires passport scope "write".)
```

### change_tags

**Change tags**

```text
Change the workspace's tags. apply and remove put one tag on, or take it off, one contact, company,
deal, lead or project, by tag_id or by the tag_name of a tag that already exists. create coins a new
word. update renames, recolours or describes one; fields left out are unchanged. apply never creates
a tag: an unknown name is refused, a name matches case-insensitively, an archived word is refused as
archived, and the same tag twice is a conflict. Before create, read the list — a near-duplicate
splits records that belong together, and a taken name is a conflict, even a retired one, which only
Settings restores. create and update need tag grants an ordinary seat lacks. update sends no
version, so read the tag right before editing; it changes what a word is called, never what carries
it. Removing a tag that is not there succeeds. Use merge_tags to fold a duplicate word into the one
kept, and archive_record on a tag to retire it for every record. (Runs immediately; requires
passport scope "write".)
```

### check_availability

**Check calendar availability**

```text
Find candidate times for a host without booking or sending anything. Set reliable=true to check the
acting host's selected Google or Microsoft calendars and pending reservations. Otherwise only
meetings recorded in this CRM are considered. calendar_backing=calendar is the evidence that live
occupancy was checked. This never checks invitees' availability. Use invite_meeting to request an
approved calendar invitation after a time is agreed. book_meeting only records a meeting.
prep_for_meeting prepares for an existing meeting. Keep the exact start and end, the
calendar_backing value, and the truncated flag. Availability is checked again when reserving the
time.
```

### commit_import

**Commit an import**

```text
Write a checked import into the workspace. The dry run is the check; this starts the write and
answers at once with the run running. Only from awaiting_approval, the state a run reaches by
producing a dry-run report, so there is always a report first, or from a failed commit, which
resumes at the row it stopped at. This cannot be undone from here — undoing an import needs the
web app. A request to get a file in is not a review of it: unless the asker says they have already
been through these rows, show the counts and the mapping and wait for their go-ahead. Use
read_import for the report first: numbers nobody read are not a check. The write goes on after this
answers. Read read_import's report until its state is complete or failed, then report what landed:
the counts and the mapping the run used, each column by the field it went to. (Runs immediately;
requires passport scope "write".)
```

### compose_analytics_report

**Compose an analytics report**

```text
WRITE a DOCUMENT somebody keeps and reads — a board-pack section, a summary for a meeting —
whose numbers come from governed queries, standard metrics or frozen editions instead of being
typed. Not for answering with a figure: a number in the reply is run_analytics_query's or
run_report's. A query-cell figure comes from run_analytics_query with save; a run_report answer has
no run id and cannot be cited. The document carries the STRUCTURE and the WORDS; each figure names a
query cell, metric_ref or edition_ref, and the server resolves those handles under the reader's own
authority. It writes no number of its own and refuses any document that does. A block carrying a
literal figure is refused EVEN WHEN a valid handle sits beside it: the literal is what renders, the
two can disagree, and no reader could tell the page shows a figure the database never computed.
Preserve returned coverage, units and capture context. For a query cell, run an analytics query with
save, and cite the run id it answers with. Ask run_analytics_query for one number when a figure is
what is wanted. This composes a DOCUMENT of several, which is worth the round trip only when the
answer is a report somebody reads. describe_vocabulary (topic report_blocks) holds the block kinds
and their fields for a caller that wants them before composing. Never put a number in a block —
cite the cell that holds it. A block kind outside the grammar is refused BY NAME with the whole set,
so a first attempt costs one refusal rather than a lookup.
```

### create_record

**Create a record**

```text
Create a contact, company, deal, lead, project, activity or relationship that does not exist yet.
Creating a deal requires a pipeline_id and a stage_id, and read_directory (list: pipelines) is what
yields them for a deal that does not exist yet. Only the fields the chosen record_type actually
stores are accepted, and a field belonging to a neighbouring type is refused rather than dropped. A
CONTACT created here is visible to the human you are acting for and to nobody else, until they
publish it or correspondence with that address earns a widening verdict — attending a meeting
together does not earn one. Do not tell anyone a contact you just created is on their colleagues'
screens. Search first when the record might already exist — a second copy of a contact or company
is a problem that then needs resolve_duplicates to undo. A record that shares only a name, with a
different email, phone or employer, is not proof of a duplicate: create it — the create files the
pair for review, and you report the pair as filed, unresolved until the reviewer decides. Several
contacts, companies or leads at once — a pasted list, a CSV — go through preview_import, which
checks every row and writes nothing; creating them one by one skips that check. The new record's id
comes back in the result; keep it for anything that links to it. When the user says where they met
the contact, pass that word as offer_tag: it only offers the tag, and change_tags still needs their
yes. (Runs immediately; requires passport scope "write".)
```

Renders its result in [`ui://margince/create-followups.html`](#create_followups_view), visible to `model`.

### create_task

**Create a task**

```text
Put a to-do on someone's list: what is owed, by whom, on which records. Creates the task only — no
reminder, no deal move; unlinked, it sits on no timeline. log_activity is for what already happened.
(Runs immediately; requires passport scope "write".)
```

### decide_approvals

**Approve or reject staged actions**

```text
Answer staged actions for the colleague asking you: approve, which lets the change happen, or
reject, which discards it. Name staged_action_id for one action, or bundle_id for every
still-waiting proposal one act staged together. The verdict is theirs — take an explicit approve
or reject rather than deciding what they would have wanted. Approving is what makes the change real,
including sending a message that was only drafted; a rejection cannot be taken back. You may approve
your own proposal only when it is a change that would have gone straight through but for a human's
earlier edit — no deal close, no relink, no tag merge, no schema change, no send; the rest is
theirs to release in the CRM. An item already answered, or lapsed, is reported as such and nothing
is written; each bundle member is answered and reported on its own, and a bundle holding none of
theirs reads as not found. read_approval when they have not seen what it holds; list_approvals is
where both ids come from. If the proposal is your OWN refused call, approving does not perform it
— re-issue that same call with approval_id set. (Runs immediately; requires passport scope
"write".)
```

### describe_vocabulary

**Describe what a tool accepts**

```text
Answer the names another tool accepts, one topic at a time. query: the record types, fields,
operators and the one relationship hop a query_workspace plan may name. analytics: the populations,
group_by dimensions, measures and functions run_analytics_query takes for this seat. record_fields:
what a create_record or update_record `fields` body accepts for each record_type. report_vocabulary:
the group_by, filters and aggregates each run_report report admits. report_blocks: the block kinds a
compose_analytics_report document may contain. It returns names, never records or numbers, and none
of it is a prerequisite: each of those tools refuses an unknown name together with the accepted
list. query, analytics and record_fields are narrowed to what you may already see. Call the tool
that does the work directly when the names are already known, and read its refusal when one is
wrong. Take every name verbatim; a name outside the document is refused, not approximated. In
record_fields a key with no `?` is required, and a custom field is spelled cf_<slug>. A report
figure is cited from a saved run, never written into a block.
```

### draft_outbound

**Draft an email or follow-ups**

```text
Write messages and send none. one_message: one email — a reply to a recorded thread (activity_id),
or a FIRST message to a contact or lead (links naming the recipient and, for a contact, the company,
deal or project it is about). slipping_deals: a follow-up for each deal in a segment at once —
today `slipping` — each left on its deal's timeline. A first email waits on the recipient's page,
marked as drafted by an agent, until the human you act for sends it there; a reply, or one not_saved
because they keep an unsent draft of their own, is not saved, so show it in full. slipping_deals
covers the deals whats_slipping_this_week reports, at most 25. send_outbound sends an email once it
is written. Keep the subject, body, to and the activity_id or links a message echoes; the send takes
them unchanged. Each follow-up comes back with its deal_id and draft_activity_id. (Runs immediately;
requires passport scope "draft".)
```

### enrich

**Enrich a company from its website**

```text
Learn about a company by reading its public website, and propose what was found for a human to
accept onto the record. It reaches OUTSIDE the workspace, and what it returns is a PROPOSAL —
nothing lands on the record until someone accepts it, which is the review that guards this, not an
approval on the call. Reading one page answers immediately; reading a whole site is queued and
answers with a read id rather than the content. What it finds is captured text from a third party,
not a fact this workspace has verified. Use change_lead_state to qualify a lead when the missing
values are already derivable from the record itself, which costs no external read and needs no
approval. Keep the company_id you enriched, and the read id when a whole-site read was queued —
the result is collected against it later. (Governance: a human approves every call before it runs;
requires passport scope "enrich".)
```

### invite_meeting

**Send a calendar invitation**

```text
Create a real calendar invitation for one contact and notify their chosen address. Requires a
connected writable calendar and a contact email the acting host may use. It checks actual busy time
and returns pending until the provider confirms. Does not prove the guest agreed or received the
notification. Use book_meeting only to record a meeting without inviting anyone. Use
check_availability with reliable=true to propose calendar-checked times. Keep the invitation id and
inspect its status before claiming it is booked. Retrying an uncertain delivery must use the
existing invitation. (Governance: a human approves every call before it runs; requires passport
scope "send".)
```

### list_approvals

**List what is waiting for a decision**

```text
The staged actions waiting for a human's decision: what was proposed and what each would do. It is
where a proposal that is already waiting turns up — a message staged and unsent is not one that
needs writing again. It lists what the colleague you act for could decide themselves; anything else
is absent rather than refused. A proposal past its expiry reads as expired and can no longer be
answered. Each item carries its one-line summary, not the change itself. read_approval opens one and
shows what it holds; decide_approvals answers it. Keep the staged_action_id you mean to act on, the
bundle_id when one act staged several, and next_cursor.
```

### list_documents

**List a record's documents**

```text
List the files stored on a company, contact, deal, lead, project or activity, newest first: name,
type, size, who added them and the contract each is filed against. It lists and never returns a
file's contents. Check it before attaching a file again, so the record does not carry the same file
twice. Keep next_cursor to read the next page.
```

### list_records

**List records**

```text
Enumerate the contacts, companies, deals, leads or projects that meet exact conditions — every
deal in one pipeline, the leads one rep owns, the projects still being delivered. It narrows only by
the filters this workspace publishes for that record_type, which the schema lists per type, and it
answers ONE page: the set continues past it. Use search_records when the question is what a record
is called rather than which records meet a condition, and run_report when the answer is a count or a
total rather than the records themselves. Keep next_cursor and pass it back to read the next page
— a second call without it re-reads the first one. A result's `owner` says who holds it. When
`owner.is_you` is false, say whose it is when you report the record (or that its owner could not be
named, when `owner.name` is absent), and treat contacting it as theirs to decide rather than
advising an approach as though the record were unowned.
```

### log_activity

**Log an activity**

```text
Record something that happened — a call, a meeting, a note, a message — on the records it was
about: name every one of them in this call. A meeting is with a contact, and also concerns their
company and the deal it is for. It writes history and changes nothing else: no deal moves, no field
updates, nobody is notified. Unlinked, it appears on no timeline, and adding a link afterwards is a
second call — relink_activity — which waits for the user's yes when it files under a project.
Use progress_deal when the same event also moves a deal, so move and note are one act; create_task
for something still owed; attach_document for a file, whose text never goes into a note. Keep the
activity id — draft_outbound and send_outbound identify a conversation by it. (Runs immediately;
requires passport scope "write".)
```

### merge_tags

**Fold one tag into another**

```text
Fold a duplicate word into the one the workspace keeps, moving every record that carries it. NOT
UNDOABLE once approved: the source is retired, its name is released — links to it stop working and
someone may coin it again — and no pointer home is kept, unlike a contact or company merge. The
TARGET is the word that survives; read both with read_tags first. Needs the tag.update grant.
(Governance: a human approves every call before it runs; requires passport scope "write".)
```

### prep_for_meeting

**Prepare for a meeting**

```text
Get ready for a specific meeting: given the meeting, the same written brief a human reads; given any
other record, the assembled picture a catch-up gives, plus the open items pulled out as the things
to raise. It is built around ONE record you name, and everything it reports carries a source; what
cannot be evidenced is absent rather than inferred. Given a meeting it works out which record that
meeting is about and names the others alongside. Use catch_me_up_on when there is no meeting and the
question is simply what has been happening, and check_availability when the goal is finding a time
rather than preparing for one. The focus list names the open items by record_id; those are what to
act on after the meeting. prepared_for names the record the prep was built around. occurred_at is
when an item happened, in UTC — prefer it over a date the prose recalls.
```

### preview_import

**Preview an import**

```text
Bring a spreadsheet in: send the CSV as text with a `mapping` saying what each column is, and this
checks every row against the workspace and reports what importing it would do. Writes nothing.
`object` is company, contact or lead. Use `contact` for a file the business already knows — a
migration off another CRM, a corrected export coming back. Use `lead` for a machine-sourced list
nobody has worked yet; those land unworked and a human promotes them. A row naming a record already
here is counted in `duplicates`, and created unless on_duplicate is skip — except a contact whose
email is already held, which is always refused, because an email is a real key. A company's Website
or Domain column maps to `domain`, which is what identifies a company — import it and dedupe stops
guessing from names. To link contacts to their employers, map the company column to `company_name`
— import the companies FIRST, because a name that matches nothing links nothing and says so. To
CORRECT companies rather than add them, map a column to `id`, then give a row the id of the company
it corrects — read them out first. A row whose `id` is EMPTY is a new company, so one file may
both correct and add. Files that link to each other go together in `files`, at most one per type,
instead of `object` and `csv`: a `ref` column is a row's own key, and a link such as a contact's
`company_ref` names a company's ref. read_import with view format lists each file's columns.
create_record for one record you already know. Keep the run_id. The counts it answers — created,
duplicates, skipped — and the mapping it settled on are what the user weighs, so report both: a
column this placed by a name they did not write is a decision they did not make. (Runs immediately;
requires passport scope "write".)
```

### progress_deal

**Progress a deal with a note**

```text
Move a deal to a different stage of its pipeline, optionally leaving a note on its timeline saying
why, in one call. The stage is named by id from read_directory (list: pipelines) — call it first;
a deal you read carries only its current stage. Moving onto or off a won or lost stage is a human's
decision: staged for approval, with a lost_reason for a losing stage. Read the target stage's
semantic rather than guessing from its name. The move commits first and the note follows it, so a
note that fails to write does not put the deal back — the answer says so, and the note is then
log_activity's to retry. Use log_activity when something happened but the deal did not move. Send
if_version with the version you read of the deal; keep the staged approval id if a closing move is
sent for approval. (Governance: some calls run immediately and others a human approves first,
decided per call from its arguments; requires passport scope "write".)
```

### query_workspace

**Query the workspace**

```text
Answer a question that has STRUCTURE — a record type, conditions on its fields, a hop to a related
record, or a likeness to describe — by sending a plan and reading back the records that satisfy
it, together with what kind of answer it is. It also finds where records are: an address is filtered
by its city (address.city) or by distance (within_radius). Every name in a plan comes from the
published vocabulary; one outside it is refused by name. The margince://schema/query resource —
not this description — says which record types, fields, operators and relationships can be asked
about. At most one similarity clause and one hop. It cannot group, count or total, and has no
cursor: an answer that hit its limit says so. Use search_records when you only have a name or a
phrase and no conditions to apply, and run_report when the answer wanted is a count, a total or a
breakdown rather than the records themselves. Read `coverage` before you use the rows:
`complete_exact` means every record matching the plan is here, `ranked_semantic` means these ranked
highest and others may match, and `partial_degraded` means something in the plan could not be
answered as asked — `notes` says which. Keep each row's record_type and id for any follow-up call,
and its `evidence` for the related record that admitted it. A row's `owner` is the colleague who
holds that company: rows come back from across the whole workspace, so most of them belong to
someone other than the contact asking. When `owner.is_you` is false, say whose it is when you report
the record (or that its owner could not be named, when `owner.name` is absent), and treat contacting
it as theirs to decide rather than advising an approach as though the record were unowned.
```

### read_approval

**Read one staged action in full**

```text
Read one staged action in full: the exact change proposed, the record it acts on, and the evidence
it was formed on — enough to answer it without opening the app. Reading performs nothing. An id
the colleague you act for could not decide answers as not found, exactly as an id naming nothing
does. list_approvals yields the id; decide_approvals answers it. Keep the staged_action_id, and the
bundle_id if the item names one.
```

Renders its result in [`ui://margince/approval.html`](#approval_view), visible to `model`.

### read_brief

**Read the morning brief**

```text
Read the ranked queue the user you act for sees when they open their morning brief — the deals the
workspace decided are worth their attention today, in order, with the rows behind each ranking. It
re-reads the last assembled run rather than building a new one, so its as_of says how current it is,
and it is that user's own queue: it cannot be asked for anyone else's. Acting on, dismissing or
snoozing an item is theirs alone. Use whats_slipping_this_week when the question is which deals are
losing momentum regardless of what today's brief chose, and read_record for what one of these deals
currently says. Each item names a deal_id and its evidence_ids; read those to cite what the ranking
rested on rather than restating the item's own summary.
```

### read_directory

**Read the workspace's colleagues, pipelines and channels**

```text
Read how this workspace is set up. colleagues: who works HERE, the seats that can receive work, not
the contacts stored as records; `q` narrows by name. pipelines: every pipeline with its live stages,
each open, won or lost. channel_providers: the messaging transports this installation composed. A
`q` matching nobody answers with `all_colleagues` and a warning, which decides whether someone truly
has no seat. A deal cannot be created, or moved anywhere new, without pipelines: a deal you read
carries only its current stage, and a stage's semantic, not its name, says whether it closes the
deal. supplies_transport=false means a reply on that transport will be refused. search_records finds
a CUSTOMER contact; this finds a colleague. user_id is what assignee_id and owner_id take; never
assign to an is_agent seat. Keep the pipeline_id and stage_id: create_record for a deal needs both,
and progress_deal takes the stage_id as to_stage_id. Carry `provider` verbatim: log_activity takes
it as channel_provider when kind is "message".
```

### read_forecast

**Read the forecast and how it moved**

```text
Read the forecast under the installation's fiscal calendar and base currency. readings: what a
period is expected to close — `won`, `evidence`, `best_case`, `open` and `weighted` — and
`snapshots`, the period's frozen states, newest first. movement: why the forecast changed between
two snapshots, as named causes that sum to the whole difference. `won` counts deals by the day they
ACTUALLY closed. `evidence` is committed pipeline whose close date somebody confirmed.
`coverage_note` says what the totals leave out; quote it with any total. movement needs `from` then
`to`, both from ONE readings answer's `snapshots`; none listed means nothing was frozen yet. Reading
twice and subtracting is not the same answer: it says nothing of where the difference went. In
movement, `definition` means the snapshots used different rules and `model` a re-scored probability:
neither is sales movement. run_report and query_workspace also produce numbers, and neither is the
forecast. For a board-pack section, run_analytics_query with save, then compose_analytics_report;
read_input_checks says whether the inputs were read. Quote `as_of`, `timezone` and `base_currency`
with the number. Every `_minor` figure is in minor units of `base_currency`: 3100000 in EUR is EUR
31,000.
```

### read_import

**Read an import run or the import format**

```text
Read one import run, or the format an import takes. report, by run_id: the run's state and files,
and once validated what it will do or did, in rows created, updated, failed, unusable and
duplicates. format: each file's columns, their order and the key rules. The report's counts are what
the user weighs before committing. After a commit, read report again until the state is complete or
failed.
```

### read_input_checks

**Can the forecast's inputs be trusted**

```text
Answer whether the forecast's inputs can be trusted, in three views. summary: the verdict —
whether the numbers are sound enough to quote — and how much of the pipeline last night's check
reached. exceptions: the open input problems themselves, most material first — a close date that
went by, an amount that disagrees with the sent offer, a deal nobody has heard from in ninety days.
sources: which connectors the nightly check could read at all, and how far back each reaches.
`checks_incomplete` is NOT a worse `needs_review`: one says the pipeline has problems, the other
says nobody could look. exceptions is scoped to what you can open, and an empty list cannot be told
apart from a clean pipeline — sources is what tells them apart. sources needs a coverage grant
that operators hold and sellers do not, so a refusal there is a seat boundary, not a missing run.
Read summary's `readiness` before quoting any forecast figure. Only a `checked` source carries a
date; any other state means nothing was read from it. In exceptions, `affected_minor` absent means
the money at stake cannot be said, not that nothing is.
```

### read_lists

**Find and read lists**

```text
Find the team's Live Lists (saved filters whose members join and leave on their own) and Shortlists
(records chosen by hand), read one, page through its members, say why a record is or is not on it,
read what changed on it — including which records a Live List was seen to gain and lose, and how
many since the user last opened it — or preview what a filter would select before a Live List is
saved. Every count, member and reason is what the user you act for may see: a list shared with them
never shows a member record they cannot read, so two users may see different counts for one list.
Live Lists are checked every 15 minutes, longest-unchecked first, so with very many lists one can
wait longer (last_check says when); a record that joined and left between two checks is not
recorded. A preview is logged as a read of those records. A list with health retired_field still
works but filters on a retired custom field, named in retired_fields; its steward should replace
that clause. A list with health retired_tag filters on an archived or merged-away tag, named in
retired_tags, and that tag no longer matches any record, so its steward should name the live tag.
search_records finds records by name; tags are applied with change_tags, not lists. Keep list_id,
the version for a later change, and next_cursor to read the next page.
```

### read_project

**Read a project or its handover**

```text
Read one project. overview: its whole page — company, phase history with time per phase, deals,
stakeholders, contracts, documents, open commitments, timeline, filing coverage, totals. handoff:
what the delivery side needs from sales — who owns it, who to call at the client, what was sold,
by when, what is already promised — with a named gap for each the records do not answer. overview
cuts each section at 25 rows and flags it truncated; sections_omitted names what your grants
withhold. A handoff gap means the field is empty as far as the caller can see. Preparing a handover
is not performing one. read_record for the project's stored fields alone, catch_me_up_on for what
has been happening. The project_id, each gap's source field, and the deal, contact and task ids a
follow-up acts on.
```

### read_record

**Read a record**

```text
Read one record's own stored fields — the values a reader would see on its detail page — when
you already know which record you mean. It returns that record and nothing around it: no timeline,
no related contacts, no deals on the company. Use catch_me_up_on when the goal is what has been
happening on the record rather than what it currently says. Keep the version from the result and
pass it back as if_version on a later update, so a write is refused rather than silently overwriting
a change made in between. Its `owner` says who holds it. When `owner.is_you` is false, say whose it
is when you report the record (or that its owner could not be named, when `owner.name` is absent),
and treat contacting it as theirs to decide rather than advising an approach as though the record
were unowned.
```

### read_relationships

**Read who we know, and how well**

```text
Read the relationships this workspace has recorded, from the one id you name. contact_id: the
colleagues who know that contact, warmest first. company_id: a warm route into the company — who
we know there and which colleague could introduce us. deal_id: which roles at the deal's company we
have a relationship with, and where the deal rests on a single contact. Only interactions somebody
recorded count, so an unlogged relationship is absent, and never spoken is no relationship rather
than a score of zero. A company nobody here has spoken to has no warm path, and saying so is the
answer. Deal coverage says nothing about whether the deal will close. at_risk_relationships finds
the thin relationships across every open deal at once. Keep the user_id of each colleague and the
contact ids a path names. For a deal, say WHO fills an uncovered seat by `contact_name`; a seat with
no name is one this caller may not read, so report the gap and do not guess.
```

### read_reporting

**Read sales reporting**

```text
Discover standard sales metrics, evaluate the same graphs as Analytics, or reopen saved report
definitions and frozen editions. Read only. Catalog returns permitted metric IDs and blocks.
Evaluate requires a selection. Report and editions require a report id; edition requires an edition
id. Evidence requires the evaluation receipt and selection, or an edition id, plus its exact
reference. Compare requires two edition IDs. Always retain coverage and capture times when quoting
numbers. Use read_directory (list: pipelines) to discover pipeline and stage IDs. Use
run_analytics_query for custom groupings and compose_analytics_report for a document. Configure
targets, report sharing and schedules in Analytics.
```

### read_tags

**Read tags**

```text
Read the workspace's tags. list: every word, with the tag_id the other tag calls take. get: one tag
and how many contacts, companies, deals and leads carry it — the records themselves come from
list_records. record: the tags on one contact, company, deal or lead, with who applied each and
when. list returns archived words only with include_archived, and they cannot be applied;
`truncated` means a word missing from the list may still exist. In record, `withheld` true means the
vocabulary is hidden from this caller, NOT that the record has no tags — never report it as none.
A word the user says fits a contact you are creating goes to create_record as offer_tag, which
offers it without applying it.
```

### relink_activities

**Re-associate a set of activities to a record**

```text
Move up to 500 named activities onto one record, all or nothing. Each id must be visible and
writable to you. A project destination, or any move the installation's policy raises, is staged for
confirmation and the retry moves exactly these ids; once the user says yes, relay it with
decide_approvals. A filing under a project can be undone by a member from the activity.
relink_activity moves one message. The answer is the count moved. (Governance: some calls run
immediately and others a human approves first, decided per call from its arguments; requires
passport scope "write".)
```

### relink_activity

**Re-associate an activity to a record**

```text
Fix what a recorded activity is about, when a captured mail or meeting landed on the wrong record or
on none. Changes only the association; content is untouched. By default the new link is ADDED beside
existing ones. Onto a project it waits for the user's yes; the answer says whether you may relay it
with decide_approvals. log_activity records an event not recorded yet; relink_activities moves a
picked set, such as a whole thread's activities. Set replace_existing_of_type to move rather than
associate. (Governance: some calls run immediately and others a human approves first, decided per
call from its arguments; requires passport scope "write".)
```

### resolve_duplicates

**Merge two records or dismiss a flagged pair**

```text
Settle two records that may be one. With candidate_id and decision: a pair create_record flagged in
duplicate_candidates — not_the_same says they are two, so the pair is not flagged again, and
reopen takes that back; neither record changes. With record_type, source_id and target_id: merge two
contacts or two companies into one, moving the source's activities, deals and links onto the
survivor. A merge archives the source and redirects it to the target; it cannot be reversed by
merging the other way round, and by default nothing holds it. A merged pair cannot be reopened. Use
archive_record when the extra record has nothing worth keeping. target_id is the record that
survives and source_id the one merged away — read both records before choosing. (Runs immediately;
requires passport scope "write".)
```

### resolve_entities

**Resolve contacts and companies**

```text
Find out whether the contacts and companies named in something you are holding already exist here,
matched on addresses, phone numbers and company domains rather than on text. It reads only. Nothing
is created, changed or merged, and it answers contact and company, never leads. A near match comes
back `ambiguous` however close it is. Use search_records to find a record you know exists, and
resolve_duplicates once a contact has decided that two records are one. Call this BEFORE creating a
contact or company from anything you did not type. Act on `matched`; on `ambiguous` ask which is
meant; on `unresolved` say what you will create — a miss is not proof nothing exists.
```

### review_commitments

**Review open commitments**

```text
Answer "what have we promised and not delivered?": the open promises across the workspace, most
overdue first, from BOTH places a promise is recorded — a task somebody filed, and a commitment
read out of a captured conversation, which carries the sentence it was read from. Each names when it
came due and the record it was made about. It reads what the workspace captured: a promise made in
an uncaptured call, or in a thread nobody filed, is absent. The two sources are not linked, so a
promise both said and typed can appear twice. Narrowing by assignee or project returns recorded
TASKS alone — a conversation commitment carries neither — so a narrowed answer is a smaller
question than the unnarrowed one. It is scoped to the records the caller may see. Use
whats_slipping_this_week when the question is which DEALS are at risk rather than which promises are
outstanding, and catch_me_up_on for everything that has happened on one record. Each item carries
source (task | conversation) and the id for that source — task_id or claim_id — plus assignee_id
where a task has one. Every state is judged against as_of, so carry that too if you report the
answer later.
```

### run_analytics_query

**Run an analytics query**

```text
Compute a grouped aggregate — counts, sums, averages, medians — over a governed population, in
the database. The answer carries its columns, rows and schema version; groups too small to disclose
are withheld, never estimated. Populations, dimensions and measures come from
margince://schema/analytics, derived for this seat and answered by describe_vocabulary (topic
analytics); a name outside it is refused with what would work. Money measures are minor units. An
omitted scope is this seat's own default population, never the workspace. run_report answers a
prebuilt report by key; query_workspace lists exact records; the forecast tools answer forecast
readings and movement. This one is for a novel aggregate no prebuilt report shapes, and for an
ad-hoc figure a compose_analytics_report document will cite as a query cell: only a saved run can be
cited. Set save to get a run_id whose cells compose_analytics_report can cite; without it the answer
is served once and not stored.
```

### run_report

**Run a report**

```text
Answer a question about totals, counts or breakdowns by running one of this workspace's prebuilt
reports. Only the named reports exist, each with its own filter, grouping and measure names;
anything else is refused. It aggregates: how many and how much, never which record. Use
search_records or whats_slipping_this_week when the answer is the records rather than a number over
them. Reach for run_analytics_query only when NO prebuilt report answers the question — a report
is one call, a query is a lookup and a call — or when the figures go into a written document: a
report's answer has no run_id, so compose_analytics_report cannot cite it. Call a report with no
plan first for its default answer, then narrow with the names describe_vocabulary gives.
```

### search_context

**Search for relevant material**

```text
Find the records most relevant to a description, ranked by meaning as well as by wording, each with
the excerpt that ranked it. Ranked, never exhaustive: records that also match may be absent, and no
count of them exists. You can narrow it to particular record types, but not by field, date or owner,
and it does not group or total. It cannot be narrowed to a project either: the index carries no
project column. Use catch_me_up_on with project_id for one project, query_workspace when the
question has conditions, a date bound or a related record to reach through, and search_records when
you have the exact name or phrase. Read `coverage`: `partial_degraded` means `notes` matters, and
`semantic_ranking_degraded_to_lexical` there means the ranking fell back to word overlap. Keep each
hit's record_type and id.
```

### search_records

**Search records**

```text
Find contacts, companies, deals, leads and projects when you know roughly what they are called but
not which record they are. It matches text stored ON the record. It does not read a timeline:
message bodies, call notes and meeting content are not searched, so a query describing what someone
said or did will not find them. Use list_records when the question is which records meet a condition
rather than what one is called, read_record when you already hold the record's id, and run_report
when the question is a count, a total or a breakdown rather than a set of records. Keep each
result's record_type and id together: every other tool identifies a record by both, and an id alone
does not say which type it belongs to. A result's `owner` says who holds it. When `owner.is_you` is
false, say whose it is when you report the record (or that its owner could not be named, when
`owner.name` is absent), and treat contacting it as theirs to decide rather than advising an
approach as though the record were unowned.
```

### search_report_evidence

**Search the evidence behind a saved run**

```text
Check a claim against the records behind a saved analytics run — or one cell of it — by
searching their text: records that carry the words are citations, records that do not are
counterexamples, and records with no text to judge are abstentions. It searches only the run's own
records that this seat can read today, and every figure counts those — never records hidden from
this seat. It states a prevalence only when coverage is complete_exact; otherwise prevalence is null
and the notes say why. search_context sweeps the whole workspace by meaning; run_analytics_query
counts. This one answers how much of a counted set supports a claim. Cite records by id. Quote a
share only from prevalence, never by dividing the lists.
```

### send_outbound

**Send a message**

```text
Put a message on the wire to a real recipient, exactly as given; it composes nothing. email_reply: a
mail answering a thread recorded here, by activity_id. email_new: a first mail starting a new
conversation, filed on `links` naming the records it is about. chat_reply: an answer on a captured
chat conversation, by activity_id, on the channel it came from. Every recipient must have granted
the consent purpose the call names. A sent message cannot be recalled, and by default nothing holds
it: where an installation has raised a send to confirm first, the answer is a staged approval
instead. chat_reply cannot start a conversation or choose a channel. Use draft_outbound first when
the message does not exist yet, and log_activity to record that something was said rather than say
it. Send the subject, body and activity_id or links the draft produced, and keep the staged approval
id: the approval binds that exact message, so changed text needs a new one. email_new answers with
the new conversation's activity_id. (Runs immediately; requires passport scope "send".)
```

### update_record

**Update a record**

```text
Change stored field values on a record that already exists — a corrected title, an amount, an
expected close date. Only the fields you send change, and only the fields the record type stores (a
contact's email addresses are not among them). A field a HUMAN last set is not overwritten: that
part is staged for a human and named in the result, and that part of the write has not happened. It
names the record by id; when a name matches two records, a human picks. owner_id is NOT neutral —
ownership decides visibility, so reassigning moves the record onto someone else's book and can take
it off the owner's. Use progress_deal to move a deal between stages, and relink_activity to change
what an activity is about; neither is a field edit. Send if_version with the version you read, and
keep the staged approval id from the result if you intend to retry the same change once a human has
released it. (Runs immediately; requires passport scope "write".)
```

Renders its result in [`ui://margince/field-conflict.html`](#field_conflict_view), visible to `model`, `app`.

### whats_slipping_this_week

**What's slipping this week**

```text
Answer "what is slipping?": the deals going quiet or running past their expected close date, ranked
worst first, each with the evidence that says so. It reports only deals whose risk can be evidenced
from their own fields — a deal nobody can point at a reason for is absent rather than guessed —
and it is scoped to the deals the caller may see. Use run_report for the pipeline as a whole
(totals, counts, breakdowns), and at_risk_relationships when the question is who a deal rests on
rather than whether it is moving. Keep each deal_id if you intend to act; draft_outbound (for:
slipping_deals) works over this same ranked set at the default quiet_days without you re-deriving
it.
```

### whoami

**Who this passport acts for**

```text
Name the human this passport acts for: their id, display name, email and language. It reads only,
and answers this call's acting user — not a directory. acting_user_id is what owner_id and
assignee_id take for "me". prose_language is the language every stored sentence is written in — a
note, a description, a summary — whatever language the conversation itself is in; it is always
answered, where locale is absent until this contact chooses one.
```

