# The end state an MCP use case reads back

A scenario of the MCP end-to-end lane can name the state its run must leave behind, under
`must_end_with`. This page lists the forms an entry takes. How to run the lane is in
[test-the-mcp-surface-end-to-end.md](../how-to/test-the-mcp-surface-end-to-end.md).

A case that writes can name the state it must leave behind:

```yaml
must_end_with:
  - company "Emsland Ventilbau GmbH" lifecycle=prospect
```

After each run, `e2e/llm/endstate.py` finds the record by its type and its whole name, as the app shows it
(every page of `list_records`). It does this before the lane puts back the snapshot.

It reads the field with `read_record`, through the passport of the lane, and compares. A wrong value, a
missing record, or two records with one name fail the run, such as
`ended with company "Emsland Ventilbau GmbH" lifecycle=target, wanted prospect`.
A read that cannot be made (an error, a field the read does not carry, or a reader that fails) is a
harness stop. The outcome goes at the end of the transcript as an `end_state` event. A `company`,
`contact`, `deal`, `deal_room`, `lead` or `project` can be read by name; another type is one line in
`endstate.py`. A field that `read_record` leaves out while it is empty, such as a contact's `title`,
reads as `(empty)` and fails the run.

The reader calls only the tools the server lists, and a router with its view. A call by the name a
router sends work on to is logged as a call to a tool that is not listed. Each call is made once per
read, because the server limits how often a passport calls. If the run called `commit_import`, the
reader first waits up to 90 seconds for that import to stop running. An import that is still running
after that is a harness stop.

Five more names read something other than a stored field. `records=N` counts
the records of that type with that whole name. `owner=` and `stage=` compare
names, not ids, so they hold on any seed. `tagged=` and `untagged=` read the
words that `read_tags` lists on the record, and a list the seat cannot see is
a harness stop. `tag "Name" records=N` counts the contacts, companies, deals
and leads that carry the word. A lead made a contact, or disqualified, leaves
`list_records`, so an entry tells the two apart with `records` on the lead and
on the contact.

`own_draft=` holds when the user's own draft on the record has that subject; an
agent's draft there fails it. `automation "Name"` reads a rule, with `owner=` as
a name or `(none)`. `me "email"` reads the lane's own seat through `whoami`.

`e2e/llm/endstate_dry.py --mcp-url URL --token-env NAME` reads every scenario
with `must_end_with` against a seeded stack, with no model. Nothing has changed
the seed yet, so each scenario exits 0 or 4. An exit of 3 is a bug in the
reader, and the script then exits 1.

An entry names an activity by its whole subject on a project, a company or a
contact:

```yaml
must_end_with:
  - activity "Vertragsunterzeichnung Leitstand Goslar" on project "Leitstand Goslar" documents=1
  - activity "Re: Lieferverzug Kolbenpumpe KP-300" on company "Rhön Hydraulik AG" records=2
```

The reader finds the parent by name. On a project it finds the activity on the timeline in
`read_project` with `view: overview`. A timeline that read cuts short is a harness stop. On a
company or a contact it reads every page of `find_records` with `engine: list`, filtered to
that record. `records=N` counts the activities with that subject there, so `records=0` says a
mail left the wrong record.

More names read a panel, and not a stored field:

| Entry | Holds when |
|---|---|
| `deal "Name" pipeline=Sales` | the deal is in the pipeline of that name |
| `deal "Name" decision=*cfo*` | one open sales decision on the deal matches the glob |
| `company "Name" signal=*fund*` | a signal on the company has a summary that matches the glob |
| `project "Name" health=at_risk` | the project's judged health is that state; `(none)` when nobody judged it |
| `contact "Name" consent=marketing_email:granted` | that consent purpose has that state |
| `contact "Name" stop=subject_request` | the contact has a stop of that kind |
| `contact "Name" data_subject_request=access` | one open request of that kind names the contact, by id or by one of its addresses |
| `list "Name" member=Jens Falk` | the record of that name is on the list; `nonmember=` is the reverse, and both fail when no record or two carry the name. `members=12` counts the members |
| `fx_rate "USD" rate=0.92` | the newest rate from that currency is that number. Two rates on that date stop the read |

A `me` entry reads more of the seat than `whoami` gives. `working_days=1,2,3,4` and
`working_hours=09:00-17:00` read the seat's own hours. `unread_notices=N` counts its unread
notices. `capture_excluded=<domain>` holds when the seat's capture excludes that domain.
`voice=<glob>` matches the seat's own Voice DNA preferences. `forecast_call=<minor> <currency>`
reads the seat's newest forecast call for this quarter. A glob matches whatever the case of the
letters.

Two names in that place read the record's files, and not a field:

```yaml
must_end_with:
  - company "Aachener Metallwerke GmbH" document=<sha256> *.md
  - company "Aachener Metallwerke GmbH" documents=1
```

`document` holds when one file on the Documents tab, and only one, has that checksum,
under a name that the glob matches. `documents` holds when the tab has that
many files. A count catches a refused file that comes back inside an archive, which has
new bytes and a new name. Both read every page of `list_documents`.

```yaml
must_end_with:
  - activity "Vertragsunterzeichnung Leitstand Goslar" on project "Leitstand Goslar" document_contract=<sha256> Servicevertrag Leitstand Goslar
```

A third name, `document_contract`, holds when the one file with that checksum is filed against
the contract of that whole title. The reader finds that contract in what
`read_project` lists for the entry's project. An agent never writes a contract, so a project without
one contract of that title is a harness stop.

A note that copies a file's text never shows in the answer. So
`must_not_call_with` reads each call itself. Each entry is `tool~regex`,
and it fails the run when that regex matches the arguments of a call to that
tool, as one JSON text. `must_call_matching` is the other direction: in the same
`tool~regex` form, it fails the run when no call to that tool matches.
