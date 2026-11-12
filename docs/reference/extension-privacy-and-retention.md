<!-- prose:plain -->
# Extension privacy and retention

An extension keeps its own tables. This page tracks the five features that make the data in those tables
follow the same privacy and retention rules as core data. It is for those who build or run Margince.

## The features

| Feature | What someone can do now | Page with the details | State |
|---|---|---|---|
| 1. Privacy declaration for extension data | Declare each table's personal data. Core clears, exports and guards it. | [Add an extension](../how-to/add-an-extension.md#declare-the-personal-data-in-your-tables) and [what core does](#what-core-does-with-extension-rows) | Done |
| 2. Limit raw database access | Nothing yet. | None yet | Planned |
| 3. Retention sweep and legal hold | Clear or delete extension rows with a retention policy. A legal hold on a linked record keeps them. | [Retention and legal hold](#retention-and-legal-hold) | Done |
| 4. Set-up from files | Nothing yet. | None yet | Planned |
| 5. Extension activation step | Nothing yet. | None yet | Planned |

## What core does with extension rows

A unit declares, per table, which core record each row belongs to, which columns are personal data and which
retention class holds it (`Extension.Privacy`). The retention class comes from the core's closed set. Core then
does the work itself:

| Action | What core does to extension rows |
|---|---|
| Erasure | Clears the personal columns of rows linked to the subject, to the subject's earlier lead records and to the activities it cleared. An activity held under a floor keeps its rows. |
| Retention rule that strips a contact or a lead of its name | Clears the rows linked to that contact or lead. |
| Erasure of an activity's content, by retention, by an owner, at the release of a stop on it, or by the noise sweep | Clears the rows linked to the activity. |
| Audit | Writes an `anonymize` audit entry and a `retention.applied` event for each cleared row. It then strips the before and after images from the row's earlier audit entries, and drops their events that have not been sent. |
| Subject export | Lists the declared columns of the same rows under `extension_data`. A table with no personal column is not read. |
| Delete of a contact, lead, company or activity | Nothing. Delete archives the record, and the row stays, so extension rows still name a record that answers. |
| Merge of a contact, lead or company | Moves a link column the unit also declares in `Reach` onto the record that stays. Refuses with a conflict that counts the rows of every other link to the record. Archive the record instead. Core deletes no contact or company. |
| A lead from a web form is erased because its notice failed. This is the one lead core deletes | Clears the rows linked to the lead, and to the notice mail it deletes, before the lead goes. |

A row whose link is still empty names no record, so no erasure or export can find it. A message that reached
`openchannel` and has not yet landed as an activity is such a row. The retention sweep is what removes it.

So let core hold the record, and keep in a unit only what it needs for its work:

- Keep personal data only while the unit still needs it. The `openchannel` unit keeps a message's body until
  it lands as an activity, and then clears it.
- Link a row to the core record, and keep no copy of the contact. Each outbound attempt in `openchannel`
  keeps the `activity_id` of the send, not the account it was sent to.

An erasure leaves a row alone while another record it links to is under legal hold. Its certificate does not
count such a row, the same as for an activity a hold keeps out of an erasure.

At start-up core refuses a declaration it could not carry out. A link column must be a `uuid`. Core audits
each row it clears or sweeps by its `id`. So that `id` must be a `uuid` primary key, or a `NOT NULL` `uuid` with
a `UNIQUE INDEX` on it alone.

A required personal column must have no `CHECK`, `UNIQUE`, `FOREIGN KEY` or `EXCLUDE` rule. It must also not be
read by a `UNIQUE INDEX`. The empty value core writes could break any of them.

A row that is already empty is not cleared again. So two erase paths that reach one row write one audit entry for it.

## Retention and legal hold

One retention policy, scope `extension`, covers the rows of every table that declares an `AgeColumn`. An admin
authors it on the retention settings page like any other policy.

| Policy action | What core does to a row past its window |
|---|---|
| `anonymize` | Clears its personal columns. A table with none is skipped. |
| `erase` | Deletes the row. |

Each row gets one audit entry under the policy's action and one `retention.applied` event. The entry strips the
images from the row's earlier audit entries, as for a cleared row.

A row is past its window when its age column is older than the policy's `retain_days`. The retention class of
its table must also no longer require it to be kept. For `commercial_correspondence` that is the strictest floor
in force: the current one or that of any legal entity. A unit row is not tied to the mail of one entity. A carried
floor keeps only the rows whose age column is before the change that made it shorter, as it does for mail.

A pass under an `anonymize` policy does not pick a row it already cleared. A pass under an `erase` policy can
still delete that row.

One pass takes one batch across all tables. Each table first gets the same share. What a table does not use goes
to the others, so a long backlog in one table does not hold up the next. The table that goes first changes each day.

A row has no legal hold of its own. It is kept while a contact, lead or company it links to is held. A row linked
to an activity is kept while that activity is held through any record it is linked to. Both the sweep and erasure
ask again in the statement that acts on the row.

Each row is acted on in a transaction of its own. That transaction first takes a shared lock on the records whose
hold keeps the row. Placing a hold updates the held record, so the two wait for each other. A hold placed first
makes the row's action wait, and then it counts. A hold placed while a row's action holds the lock waits for that
one row, and it keeps every later row of the same pass.

At start-up core refuses a table with personal columns and no `AgeColumn`. The `AgeColumn` must be a `NOT NULL`
`timestamptz` or `date` column: a row with no age would never be due. A foreign key may name a swept table only
with `ON DELETE CASCADE`, and so may a key that names a table such a delete reaches. With any other action an
`erase` policy could fail to delete its rows. Core refuses it and names the path to the table that holds the key.

`openchannel` counts a message's age from `updated_at`, when it was last decided about, as its own sweep does. Its
own sweep deletes a decided message after 30 days, and never one that still waits to land. The core sweep reaches
a waiting message too, but only once the policy's window and the floor of its retention class have passed.

## How to read this page

**Done** means the feature is in the code and its page is linked. **Planned** means it is not built.
