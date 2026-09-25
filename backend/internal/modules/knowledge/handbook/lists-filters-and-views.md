# Lists, filters and views

The list screens in Margince are **Contacts**, **Companies**, **Leads**,
**Deals** and **Projects** in the sidebar. This page is how you search, filter,
sort and page through them, switch between table and board, keep a view you
use every day, select several rows at once, and build a detailed filter on
**Filters and views**. Creating and editing a single record is in
[Contacts, companies, leads, deals and projects](records.md) and
[Leads, deals and projects](leads-deals-and-projects.md).

## How a list screen is laid out

Every Margince list screen has the same three rows. The header row carries the
view tabs (such as **All** and **Mine**), the count ("1 to 25 of 200
contacts") and the create button. The toolbar row carries, from left to right,
the **Search** box, the **Filter** button and any filters you applied, **Show
archived**, then **Sort**, **Display**, the **Table** / **Board** switch where
there is one, and **Save view**. The rows sit under it, with **Previous**,
**Next** and **Rows per page** at the bottom. Click a row to open the record.

What each list offers differs, and this table is the whole of it:

| List | Search | Filters | Preset tabs | Table and board | Bulk actions |
|---|---|---|---|---|---|
| **Contacts** | name, or exact email | Owner, Tags | All, Mine | table only | none |
| **Companies** | name, or exact domain | Owner, Company size, Tags, Lifecycle, Relationship type | All, Mine, Customers, Prospects | table only | none |
| **Leads** | name, exact email or LinkedIn URL | Status, Score, Response, Source, Owner | Mine, All, Unassigned, New and unassigned, New, Needs follow-up, Engaged, Hot, Overdue | both | Assign, Disqualify |
| **Deals** | no search box | Stage, Company, Tags, Stalled only, My deals, Partner-sourced, Forecast, Motion, Priority, Source, Partner | Newest | both, opens on Board | Assign, Move to stage, Archive |
| **Projects** | name or project key | Phase | All, In delivery | table only | none |

**Response** on Leads appears only while a first-response target is switched
on, and so does the **Overdue** tab. **Source** and **Partner** on Deals appear
only when there is something to pick.

## Searching and filtering a list

### How do I search a list?
To search a list in Margince, open the list in the sidebar — **Contacts**, **Companies**, **Leads** or **Projects** — and type in the **Search** box at the left of the toolbar.
1. Type part of a name. The list narrows as you type.
2. Or type a whole email address (Contacts, Leads), domain (Companies), LinkedIn URL (Leads) or project key (Projects).
3. Clear the box to see everything again.
**Deals** has no search box: find a deal with the command palette (⌘K or Ctrl+K), or filter by **Company** or **Stage**.
Also called: find a contact, look up a customer, search the table.

### How do I find a contact by email address?
To find a contact by email address in Margince, open **Contacts** in the sidebar and paste the whole address into the **Search** box; it matches the exact address, whatever its capitals.
1. Type or paste the full address, such as anna@example.com. Part of an address does not match.
2. For a lead, do the same on **Leads**; for a company, type its domain on **Companies**.
The command palette (⌘K or Ctrl+K) searches names and titles, not email addresses, so use the list's **Search** box for an address.
Also called: look up by email, search by email, who is this address, find a sender.

### How do I filter a list?
To filter a list in Margince, open the list and press **Filter** in the toolbar, pick what to filter by, then pick the value.
1. Press **Filter**; **Search attributes** narrows the choices.
2. Pick an attribute, such as **Owner** or **Tags**, then a value, such as **Owned by you**.
3. The filter stands in the toolbar as its own row ("Owner is Owned by you"). Press **+** to add another.
4. Click the value to change it; its **⋮** menu → **Delete filter** takes it off.
A row must match every filter, and each filter holds one value.
Also called: narrow down, refine, facet, filter by.

### How do I clear all the filters on a list?
To clear the filters on a Margince list, delete each filter row with its **⋮** menu → **Delete filter**, or press **All** among the view tabs. When nothing matches, the list says "No contacts match these filters." (or companies, leads, deals) with a **Clear filters** button, which removes every filter at once.
On the **Mine** tab, an empty list says "No contacts owned by you." with **Show all**, which drops only the owner filter.
Also called: reset filters, show everything again, remove filter.

### How do I see only my records, or the records I own?
To see only the records you own in Margince, open **Contacts**, **Companies** or **Leads** and press the **Mine** tab in the header row. On **Deals**, press **Filter** → **My deals** → **My deals**.
1. Or press **Filter** → **Owner** and pick **Owned by you**, one of your teams, or **Unassigned**.
2. **Leads** opens on **Mine** for a user who sees only their own leads, and on **All** for one who can see the team's or everyone's.
**Projects** has no owner filter anywhere — its only filter is **Phase**, and **Filters and views** does not cover projects.
Also called: my contacts, my deals, my accounts, assigned to me, owned by me.

### How do I filter by a colleague's name?
To filter a list by one colleague in Margince, use **Filters and views**: the owner filters on the list screens offer only you, your teams and **Unassigned**.
1. Open **Filters and views** and pick the **Record type**: **Contacts**, **Companies** or **Deals**.
2. Choose **Add clause**, then under **Select field** pick **Owner**.
3. Keep the operator **is** (or **is any of** for several colleagues) and pick the colleague.
The matching rows show under **Matching records**.
Also called: records owned by a teammate, another user's deals, filter by sales rep.

### How do I filter deals by company or stage?
To filter deals by company or stage in Margince, open **Deals**, press **Filter**, and pick **Company** or **Stage**.
1. For **Company**, type part of the company's name and pick it.
2. For **Stage**, pick a stage of the pipeline shown. The **Pipeline** selector beside **Table** / **Board** decides which stages are offered; changing it clears the stage filter.
3. **Stalled only** shows open deals with no activity for more than 60 days.
A company's own **Deals and projects** tab also lists its deals.
Also called: deals for a customer, deals for an account, deals in a stage.

### How do I filter contacts by company?
The **Contacts** list in Margince has no company filter. To see the contacts who work at a company, open the company from **Companies** and choose its **Contacts** tab.
The **Company** column on **Contacts** is sortable, so sorting by it groups contacts by their employer.
Also called: contacts at an account, everyone at a customer, staff of a client.

### How do I filter a list by tag?
To filter a list by tag in Margince, open **Contacts**, **Companies** or **Deals**, press **Filter** → **Tags**, and pick the tag.
One tag at a time: a second tag replaces the first. **Any tag** takes the filter off.
To see every record with a tag across all record types, open the tag's own page from the **Tags** panel on any record that carries it.
Also called: filter by label, show tagged records, segment by tag.

### How do I filter by date?
The list screens in Margince have no date filter. To order a list by date, sort it: **Contacts**, **Companies** and **Leads** sort by **Created** and **Last activity**; **Deals** by **Expected close** and **Last signal**; **Projects** by **Last activity**.
On **Filters and views**, the date operators **is after**, **is on or after**, **is before** and **is on or before** are offered only for date custom fields your company has added; created and expected close dates are not filter fields there.
Also called: created this month, filter by created date, date range.

### How do I find deals closing this month?
Margince has no filter on a deal's expected close date. To see the deals closing soonest, open **Deals**, switch to **Table**, and sort by **Expected close**: the earliest date comes first, and an open deal cannot carry a date in the past.
1. Narrow it with **Filter** → **Forecast** or **Stage**.
2. For how much this period will land, open **Analytics** → **Forecast** and pick the **Period**: **Month**, **Quarter** or **Week**. It counts open deals whose expected close falls in the period, but does not list them.
Also called: closing this quarter, filter by close date, deals due to close, expected close date.

## Sorting and arranging a list

### How do I sort a list, such as deals by value?
To sort deals by value in Margince, open **Deals**, switch to **Table**, and click the **Value** heading, or press **Sort** and pick **Value**; the biggest deal comes first. Every list sorts the same way.
1. Click a column heading, or press **Sort** and pick it under **Sort by**.
2. Clicking it again flips the order between ascending and descending.
3. The button then names it, for example **Sort: Value**, and the count line adds "sorted by Value".
4. **Default order** under **Sort by** returns to the list's own order.
Also called: order by, biggest deals first, sort by amount, sort A to Z, newest first.

### Which columns can I sort by?
The sortable columns on each Margince list are: **Contacts**: Name, Email, Company, Owner, Last activity, Created. **Companies**: Company, Description, Website, Contacts, Open deals, Lifecycle, Owner, Last activity, Created. **Leads**: Name, Score, Status, Next step, Last activity, Source, Owner, Created. **Deals**: Name, Company, via partner, Stage, Value, Expected close, Last signal, Status. **Projects**: Project name, Company, Phase, Owner, Last activity.
**Tags**, **Relationship type** and **Last email** cannot be sorted. A hidden column can still be picked under **Sort**.
Also called: sort options, order by column.

### How do I switch between the table and the board?
To switch between table and board in Margince, open **Deals** or **Leads** and press **Table** or **Board** in the toolbar.
- **Deals** opens on **Board**: one column per stage of the pipeline shown, and you drag a card to move the deal. See [The pipeline](the-pipeline.md).
- **Leads** opens on **Table**. Its **Board** has a column per status, and you drag a card between New, Contacted and Engaged.
Filters and search still apply on the board. Bulk selection, **Display** and **Save view** are offered on the table only.
Also called: kanban, pipeline view, list view, card view.

### How do I hide columns or make the table more compact?
To choose columns or row height in a Margince list, press **Display** in the toolbar.
1. Under **Shown columns**, untick a column to hide it. The name column always stays.
2. Under **Density**, tick **Compact** for tighter rows.
3. Drag the edge of a column heading to make it wider or narrower.
Column widths are remembered in your browser for next time. Hidden columns and density are not saved in a view.
Also called: add column, remove column, change table layout.

### How do I show more rows per page?
To show more rows on a Margince list, open **Rows per page** at the bottom of the list and pick **25 per page**, **50 per page** or **100 per page**.
Use **Previous** and **Next**, or a page number, to move through the pages. The count at the top says how many match, for example "1 to 25 of 200 contacts"; where the server does not count, it says "loaded" instead.
Also called: page size, see all records, load more.

## Archived records on a list

### How do I see archived records?
To see archived records in Margince, open the list — **Contacts**, **Companies**, **Leads**, **Deals** or **Projects** — and tick **Show archived** in the toolbar.
Archived rows then appear among the live ones with an **Archived** badge; untick it to hide them again. On **Leads**, disqualified and qualified leads are the archived ones.
An archived record opens read-only and cannot be restored from the app. See [Contacts, companies, leads, deals and projects](records.md).
Also called: show deleted, find an archived contact, closed records, inactive.

## Views

A view in Margince is a tab in a list's header row that sets the list's filters
and order in one press. Each list has preset views, such as **All** and **Mine**,
and you can add your own with **Save view**. A saved view remembers the search
text, the filters, the sort, **Show archived** and the rows per page. It does not
remember hidden columns or density.

**Saved views are private.** A view you save is yours alone: colleagues do not
see it, and you cannot see theirs. There is no way to share a view in the app
today; to show a colleague the same list, tell them which filters to apply, or
build it on **Filters and views** and export it for them.

### How do I save a view?
To save a view of a list in Margince, set the list up the way you want, press **Save view** at the right of the toolbar, give it a **Name** and press **Save**.
1. Search, filter, sort or tick **Show archived**. **Save view** appears only once the list differs from its default.
2. Press **Save view** and, in **Save this view**, type a **Name**.
3. Press **Save**. The view becomes a new tab beside the preset ones.
On **Deals**, **Save view** is on the **Table** only, and the view also keeps the pipeline.
Also called: save a filter, save a search, bookmark a list, smart list, saved list.

### How do I open a saved view?
To open a saved view in Margince, open the list it was saved on and press its tab in the header row, beside **All** and **Mine**. The list takes the saved filters, search, sort and **Show archived** at once.
A view saved on **Filters and views** is opened there instead, with **Load saved filter**.
Also called: use my saved list, go back to my filter.

### How do I rename or delete a saved view?
To rename or delete a saved view in Margince, open the list it was saved on and press **Manage views** at the right of the toolbar. **Saved views** lists each view.
1. To rename one, choose **Rename** on its row, change the **Name** and press **Save**.
2. To delete one, choose **Delete** on its row, then **Delete view**. Its tab goes; the records it listed do not change.
**Manage views** appears once you have saved a view for that list.
Also called: remove a saved view, edit a view, change a view's name.

### Can I share a saved view with my team?
No. Saved views in Margince are private to whoever saved them, and the app has no share option for a view. To give a colleague the same list, send them the filters to apply, or build the filter on **Filters and views**, press **Export CSV** and send the file.
Sharing a single record is different: see [Seats, roles and who can see what](seats-roles-and-access.md).
Also called: team view, shared list, share a filter.

## Filters and views: detailed filters

**Filters and views** in the sidebar (under **Work**) is the Margince screen for
a filter a list's toolbar cannot express: several conditions, "any of" instead of
"all of", groups inside groups, custom fields, and a colleague other than you.
It covers **Contacts**, **Companies** and **Deals**. The filter is dynamic ("Dynamic:
updates on every event"): it is re-run every time you look, so it always shows
today's matches.

### How do I build a filter?
To build a detailed filter in Margince, open **Filters and views** in the sidebar, pick the **Record type**, and choose **Add clause**.
1. Pick the **Record type**: **Contacts**, **Companies** or **Deals**.
2. Choose **Add clause**, pick a field under **Select field**, an operator (**is**, **is not**, **is any of**, **contains**…) and the value.
3. Add more. **Match mode** is **All (AND)** or **Any (OR)**; **Add group** nests a group.
4. The count ("12 contacts match") and **Matching records** update live.
Groups nest at most 4 levels deep.
Also called: advanced search, segment, query builder.

### What can I filter on in Filters and views?
The fields on **Filters and views** in Margince are, for **Contacts**: **Owner**, **Owner team**, **Tag** and custom fields. **Companies** add **Industry**, **Size**, **Lifecycle**, **Relationship type**, **Domain**, and what the company runs (**Mail system**, **Hosting**, **Operated service**, **Technology**). **Deals** add **Pipeline**, **Stage**, **Company**, **Partner**, **Project**, **Status**, **Forecast category**, **Company industry**, **Company size** and **Company lifecycle**. Custom fields carry a **Custom field** badge. Your own company is never in a company filter.
Also called: filter fields, which attributes can I filter.

### How do I save a filter from Filters and views?
To save a filter built on **Filters and views**, finish at least one clause and press **Save view** beside the match count, then give it a **Name** and press **Save**.
To use it again, open **Filters and views**, pick the same **Record type**, and choose it under **Load saved filter**.
A filter saved here does not appear as a tab on the list screens, and a list's saved view does not appear under **Load saved filter**.
Also called: save a segment, keep an advanced search.

## Selecting several records

### How do I select several records at once?
To select several records in Margince, open **Leads** or **Deals** as a **Table** and tick the checkbox at the start of each row. A bar appears above the rows saying "{count} selected" with the actions for that list.
There is no select-all box: tick each row. Only open rows have a checkbox: a closed or archived deal, and a qualified or disqualified lead, cannot be selected.
**Contacts**, **Companies** and **Projects** have no bulk selection: change those one at a time.
Also called: multi-select, bulk edit, mass update, select all.

### How do I reassign several deals or leads to a colleague?
To reassign several deals or leads at once in Margince, tick them on the **Deals** or **Leads** table, pick the new owner in the bulk bar, and press **Assign**.
1. Pick the colleague: **Pick an owner** on Deals, **Choose owner** on Leads.
2. Press **Assign**. Rows that went through leave the selection.
3. Refused rows stay ticked under "{count} not applied:" with a reason, such as "no permission to reassign". Press **Assign** to retry.
Leads assigned away leave **Mine**; the notice offers **Show all**.
Also called: bulk reassign, hand over accounts, transfer deals, a colleague left, reassign all their deals.

### What can I do to several deals or leads at once?
The bulk bar on the Margince **Deals** table offers **Assign** (a new owner), **Move to stage** (open stages only) and **Archive**; the bulk bar on the **Leads** table offers **Assign** and **Disqualify** (with a **Reason**).
Archiving deals in bulk asks "Archive {count} deals?" and warns "Archived deals leave every list and report and cannot be restored here." You cannot win or lose deals in bulk, and bulk disqualified leads are reopened one at a time.
Nothing else can be changed in bulk, such as tags or custom fields.
Also called: bulk actions, mass archive, bulk disqualify.

### Can I tag several records at once?
No. Margince has no bulk tagging: the bulk bar on the **Deals** table offers only **Assign**, **Move to stage** and **Archive**, the one on **Leads** only **Assign** and **Disqualify**, and **Contacts** and **Companies** have no bulk selection. To tag several records, open each one and choose **Add tag** in its **Tags** panel.
To work with the tagged records afterwards, use **Filter** → **Tags** on the list.
Also called: bulk tag, mass tag, tag many contacts, label several deals at once.

## Exporting a list

### How do I export a list to CSV or Excel?
To export records from Margince, build a filter on **Filters and views** and press **Export CSV** or **Export JSON**; the list screens have no export button.
1. Open **Filters and views** and pick **Contacts**, **Companies** or **Deals**.
2. Choose **Add clause** and finish at least one clause; the export buttons then appear.
3. Press **Export CSV** (opens in Excel) or **Export JSON**.
Only records you can see are exported, and each export is audited. See [What is kept, what is destroyed](retention-exports-and-deletion.md).
Also called: download a list, export to spreadsheet, extract contacts.
