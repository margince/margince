<!-- prose:plain -->
# Extension privacy and retention

An extension keeps its own tables. This page tracks the five features that make the data in those tables
follow the same privacy and retention rules as core data. It is for those who build or run Margince.

## The features

| Feature | What someone can do now | Page with the details | State |
|---|---|---|---|
| 1. Privacy declaration for extension data | Declare each table's personal data. Core clears, exports and guards it. | [Add an extension](../how-to/add-an-extension.md#declare-the-personal-data-in-your-tables) and [what core does](#what-core-does-with-extension-rows) | Done |
| 2. Limit raw database access | Nothing yet. | None yet | Planned |
| 3. Retention sweep and legal hold | Nothing yet. | None yet | Planned |
| 4. Set-up from files | Nothing yet. | None yet | Planned |
| 5. Extension activation step | Nothing yet. | None yet | Planned |

## What core does with extension rows

A unit declares, per table, which core record each row belongs to, which columns are personal data and which
retention class holds it (`Extension.Privacy`). The retention class comes from the core's closed set. Core then
does the work itself:

| Action | What core does to extension rows |
|---|---|
| Erasure | Clears the personal columns of rows linked to the subject, to the subject's earlier lead records and to the activities it cleared. An activity held under a floor keeps its rows. |
| Retention rule that strips a contact of its name | Clears the rows linked to the contact. |
| Audit | Writes an `anonymize` audit entry and a `retention.applied` event for each cleared row. The audit log then hides the before and after images of the row's earlier entries. |
| Subject export | Lists the declared columns of the same rows under `extension_data`. |
| Delete of a contact, lead, company or activity | Nothing. Delete archives the record, and the row stays, so extension rows still name a record that answers. |
| Merge of a contact, lead or company | Refuses with a conflict that counts the rows linking to it. Archive the record instead. Core deletes no contact or company. |
| A lead from a web form is erased because its notice failed. This is the one lead core deletes | Clears the rows linked to the lead, and to the notice mail it deletes, before the lead goes. |

## How to read this page

**Done** means the feature is in the code and its page is linked. **Planned** means it is not built.
