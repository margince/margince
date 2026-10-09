<!-- prose:plain -->
# Lists, filters and views

The list screens in Margince are **Contacts**, **Companies**, **Leads**, **Deals** and **Projects** in the sidebar. How to create and edit a single record is in [Contacts, companies, leads, deals and projects](records.md) and [Leads, deals and projects](leads-deals-and-projects.md).

## How a list screen looks

Every Margince list screen has the same three rows. The header row holds the view tabs (such as **All** and **Mine**), the count ("1 to 25 of 200 contacts") and the create button. The toolbar row holds, from left to right, the **Search** box, the **Filter** button and any filters you set, and **Show archived**. Then come **Sort**, **Display**, the **Table** / **Board** switch where there is one, and **Save view**. The rows sit under it, with **Previous**, **Next** and **Rows per page** at the bottom. Click a row to open the record.

What each list offers:

| List | Search | Filters | Tabs | Table and board | Bulk actions |
|---|---|---|---|---|---|
| **Contacts** | name, or exact email | Owner, Tags | All, Mine | table only | Assign owner, Add or Remove tag, Create task, Add to Shortlist, Archive |
| **Companies** | name, or exact domain | Owner, Company size, Tags, Lifecycle, Relationship type | All, Mine, Customers, Prospects | table only | Assign owner, Add or Remove tag, Create task, Add to Shortlist, Archive |
| **Leads** | name, exact email or LinkedIn URL | Status, Score, Response, Source, Owner | Mine, All, Unassigned, New and unassigned, New, Needs follow-up, Engaged, Hot, Overdue | both | Assign, Disqualify |
| **Deals** | no search box | Stage, Company, Tags, Stalled only, My deals, Partner-sourced, Forecast, Motion, Priority, Source, Partner | Newest | both, opens on Board | Assign owner, Move to stage, Add or Remove tag, Create task, Add to Shortlist, Archive |
| **Projects** | name or project key | Phase | All, In delivery | table only | none |

**Response** on Leads shows only while a target for the first answer is turned on, and so does the **Overdue** tab. **Source** and **Partner** on Deals show only when there is something to pick.

## Searching and filtering a list

### How do I search a list?
To search a list in Margince, open the list in the sidebar (**Contacts**, **Companies**, **Leads** or **Projects**). Type in the **Search** box at the left of the toolbar.
1. Type part of a name. The list gets shorter as you type.
2. Or type a whole email address (Contacts, Leads), domain (Companies), LinkedIn URL (Leads) or project key (Projects).
3. Clear the box to see everything again.

**Deals** has no search box: find a deal with the command palette (`⌘K` or `Ctrl+K`), or filter by **Company** or **Stage**.
Also called: find a contact, look up a customer, search the table.

### How do I find a contact by email address?
To find a contact by email address in Margince, open **Contacts** in the sidebar and paste the whole address into the **Search** box. It matches the exact address, in capitals or not.
1. Type or paste the full address, such as `anna@example.com`. Part of an address does not match.
2. For a lead, do the same on **Leads**; for a company, type its domain on **Companies**.
The command palette (`⌘K` or `Ctrl+K`) searches names and titles, not email addresses. So use the list's **Search** box for an address.
Also called: look up by email, search by email, who is this address, find a sender.

### How do I filter a list?
To filter a list in Margince, open the list and press **Filter** in the toolbar. Pick what to filter by, then pick the value.
1. Press **Filter**; **Search attributes** makes the list of choices shorter.
2. Pick a field, such as **Owner** or **Tags**, then a value, such as **Owned by you**.
3. The filter stands in the toolbar as its own row ("Owner is Owned by you"). Press **+** to add another.
4. Click the value to change it; its **⋮** menu → **Delete filter** takes it off.
A row must match every filter, and each filter holds one value.
Also called: narrow down, refine, facet, filter by.

### How do I clear all the filters on a list?
To clear the filters on a Margince list, delete each filter row with its **⋮** menu → **Delete filter**, or press **All** in the view tabs. When nothing matches, the list says "No contacts match these filters." (or companies, leads, deals). It shows a **Clear filters** button, which removes every filter at once.
On the **Mine** tab, an empty list says "No contacts owned by you." with **Show all**, which removes only the owner filter.
Also called: reset filters, show everything again, remove filter.

### How do I see only my records, or the records I own?
To see only the records you own in Margince, open **Contacts**, **Companies** or **Leads** and press the **Mine** tab in the header row. On **Deals**, press **Filter** → **My deals** → **My deals**.
1. Or press **Filter** → **Owner** and pick **Owned by you**, one of your teams, or **Unassigned**.
2. **Leads** opens on **Mine** for a user who sees only their own leads. It opens on **All** for one who can see the team's or everyone's.
**Projects** has no owner filter at all. Its only filter is **Phase**, and **Filters and views** does not cover projects.
Also called: my contacts, my deals, my accounts, assigned to me, owned by me.

### How do I filter by a colleague's name?
To filter a list by one colleague in Margince, use **Filters and views**. The owner filters on the list screens offer only you, your teams and **Unassigned**.
1. Open **Filters and views**, press **New filter** and pick **Contacts**, **Companies** or **Deals**.
2. Press **Add condition**, then under **Select field** pick **Owner**.
3. Keep **is** (or **is any of** for more than one colleague) and pick the colleague.
The rows that match show under **Matching records**.
Also called: records owned by a teammate, another user's deals, filter by sales rep.

### How do I filter deals by company or stage?
To filter deals by company or stage in Margince, open **Deals**, press **Filter**, and pick **Company** or **Stage**.
1. For **Company**, type part of the company's name and pick it.
2. For **Stage**, pick a stage of the pipeline shown. The **Pipeline** picker next to **Table** / **Board** decides which stages you can pick. Changing the pipeline clears the stage filter.
3. **Stalled only** shows open deals with no activity for more than 60 days.
A company's own **Deals and projects** tab also lists its deals.
Also called: deals for a customer, deals for an account, deals in a stage.

### How do I filter contacts by company?
The **Contacts** list in Margince has no company filter. To see the contacts who work at a company, open the company from **Companies** and choose its **Contacts** tab.
You can sort the **Company** column on **Contacts**, so sorting by it groups contacts by their employer.
Also called: contacts at an account, everyone at a customer, staff of a client.

### How do I filter a list by tag?
To filter a list by tag in Margince, open **Contacts**, **Companies** or **Deals**, press **Filter** → **Tags**, and pick the tag.
One tag at a time: a second tag takes the place of the first. **Any tag** takes the filter off. To see every record with a tag across all record types, open the tag's own page. Use the **Tags** panel on any record that has it.
Also called: filter by label, show tagged records, segment by tag.

### How do I filter by date?
The list screens in Margince have no date filter. To put a list in date order, sort it. **Contacts**, **Companies** and **Leads** sort by **Created** and **Last activity**; **Deals** by **Expected close** and **Last signal**; **Projects** by **Last activity**.
On **Filters and views**, there are the date choices **is after**, **is on or after**, **is before** and **is on or before**. They show only for date custom fields your company has added. Created and expected close dates are not filter fields there.

Also called: created this month, filter by created date, date range.

### How do I find deals closing this month?
Margince has no filter on a deal's expected close date. To see the deals that close soonest, open **Deals**, switch to **Table**, and sort by **Expected close**. The earliest date comes first, and an open deal cannot have a date in the past.
1. Make it shorter with **Filter** → **Forecast** or **Stage**.
2. To see what this month or quarter will bring in, open **Analytics** → **Forecast** and pick the **Period**.
Choose **Month**, **Quarter** or **Week**. It counts open deals whose expected close is in that time, but does not list them.
Also called: closing this quarter, filter by close date, deals due to close, expected close date.

## Sorting a list and choosing columns

### How do I sort a list, such as deals by value?
To sort deals by value in Margince, open **Deals**, switch to **Table**, and click the **Value** heading. Or press **Sort** and pick **Value**. The biggest deal comes first. Every list sorts the same way.
1. Click a column heading, or press **Sort** and pick it under **Sort by**.
2. Click it again to switch between ascending and descending order.
3. The button then names it, for example **Sort: Value**, and the count line adds "sorted by Value".
4. **Default order** under **Sort by** goes back to the list's own order.
Also called: order by, biggest deals first, sort by amount, sort A to Z, newest first.

### Which columns can I sort by?
The columns you can sort on each Margince list are:
- **Contacts**: Name, Email, Company, Owner, Last activity, Created.
- **Companies**: Company, Description, Website, Contacts, Open deals, Lifecycle, Owner, Last activity, Created.
- **Leads**: Name, Score, Status, Next step, Last activity, Source, Owner, Created.
- **Deals**: Name, Company, via partner, Stage, Value, Expected close, Last signal, Status.
- **Projects**: Project name, Company, Phase, Owner, Last activity.
You cannot sort by **Tags**, **Relationship type** or **Last email**. You can still pick a hidden column under **Sort**.
Also called: sort options, order by column.

### How do I switch between the table and the board?
To switch between table and board in Margince, open **Deals** or **Leads** and press **Table** or **Board** in the toolbar.
- **Deals** opens on **Board**: one column for each stage of the pipeline shown, and you drag a card to move the deal. See [The pipeline](the-pipeline.md).
- **Leads** opens on **Table**. Its **Board** has a column for each status, and you drag a card between New, Contacted and Engaged.
Filters and search still work on the board. Ticking many rows, **Display** and **Save view** are on the table only.
Also called: kanban, pipeline view, list view, card view.

### How do I hide columns or make the table more compact?
To choose columns or row size in a Margince list, press **Display** in the toolbar.
1. Under **Shown columns**, untick a column to hide it. The name column always stays.
2. Under **Density**, tick **Compact** for smaller rows.
3. Drag the side of a column heading to make it wider or narrower.
Your browser keeps how wide each column is for next time. Hidden columns and density are not saved in a view.
Also called: add column, remove column, change table layout.

### How do I show more rows per page?
To show more rows on a Margince list, open **Rows per page** at the bottom of the list. Pick **25 per page**, **50 per page** or **100 per page**.
Use **Previous** and **Next**, or a page number, to move through the pages. The count at the top says how many match, for example "1 to 25 of 200 contacts". Where Margince does not have the full count, it says "loaded" instead.
Also called: page size, see all records, load more.

## Archived records on a list

### How do I see archived records?
To see archived records in Margince, open the list (**Contacts**, **Companies**, **Leads**, **Deals** or **Projects**). Tick **Show archived** in the toolbar.

Archived rows then show with the live ones with an **Archived** mark; untick it to hide them again. On **Leads**, disqualified and qualified leads are the archived ones. An archived record opens read-only. You can bring back a contact, company or deal with **Undo** on the entry that archived it, in its history. See [Contacts, companies, leads, deals and projects](records.md).
Also called: show deleted, find an archived contact, closed records, inactive.

## Views

A view in Margince is a tab in a list's header row that sets the list's filters and order in one press. Each list has its own views, such as **All** and **Mine**, and you can add your own with **Save view**. A saved view keeps the search text, the filters, the sort, **Show archived** and the rows per page. It does not keep hidden columns or density.

**Saved views are yours.** A view you save is private: colleagues do not see it, and you cannot see theirs. To give colleagues the same set of records, build the filter on **Filters and views** and save it as a **Live List**. You share that with a team or with everyone. See [Live Lists and Shortlists](#live-lists-and-shortlists).

### How do I save a view?
To save a view of a list in Margince, set the list up as you want it. Press **Save view** at the right of the toolbar. Give it a **Name** and press **Save**.
1. Search, filter, sort or tick **Show archived**. **Save view** shows only once the list differs from its default.
2. Press **Save view** and, in **Save this view**, type a **Name**.
3. Press **Save**. The view becomes a new tab next to the others.
On **Deals**, **Save view** is on the **Table** only, and the view also keeps the pipeline.
Also called: save a filter, save a search, bookmark a list, smart list, saved list.

### How do I open a saved view?
To open a saved view in Margince, open the list it was saved on. Press its tab in the header row, next to **All** and **Mine**. The list takes the saved filters, search, sort and **Show archived** at once.
A view saved on **Filters and views** opens there instead: press its name in the library.
Also called: use my saved list, go back to my filter.

### How do I rename or delete a saved view?
To rename or delete a saved view in Margince, open the list it was saved on. Press **Manage views** at the right of the toolbar. **Saved views** lists each view.
1. To rename one, choose **Rename** on its row, change the **Name** and press **Save**.
2. To delete one, choose **Delete** on its row, then **Delete view**. Its tab goes; the records it listed do not change.
**Manage views** shows once you have saved a view for that list.

A view saved on **Filters and views** has no tab. Press **⋯** on its row in the library, or next to its name once it is open. Choose **Rename**, or **Delete view** and then **Delete view** again to confirm.
Also called: remove a saved view, edit a view, change a view's name.

### Can I share a saved view with my team?
Not the view itself: a saved view in Margince stays private to whoever saved it. To share the same filter, save it as a Live List.
1. On **Filters and views**, open your saved view and choose **Save as Live List** under **⋯** next to its name. For a new filter, press **Save** and choose **Live List** under **Keep it as**.
2. Give it a **Name**, and under **Who can find it** pick **A team** or **Everyone**.
3. Press **Save list**. Your colleagues find it in the library under **Shared**.
Each colleague sees only the records they could already see. Sharing a single record is not the same: see [Seats, roles and who can see what](seats-roles-and-access.md).
Also called: team view, shared list, share a filter, share a segment.

## Filters and views: detailed filters

**Filters and views** in the sidebar (under **Work**) is the Margince screen for a filter that a list's toolbar cannot build. That means many rules, "any of" instead of "all of", groups inside groups, custom fields, and a colleague other than you. It covers **Contacts**, **Companies**, **Deals** and **Leads**. A filter runs again every time you look, so it always shows the matches of today.

The screen opens on the library of every saved view and list you can use. You can narrow it by search and by record type. **Only me** holds your saved views and the lists only you can find. **Shared** holds the lists shared with a team or with everyone. With Lists off, one group, **Saved views**, holds your views.

### How do I build a filter?
To build a detailed filter in Margince, open **Filters and views** in the sidebar. Press **New filter** and pick **Contacts**, **Companies**, **Deals** or **Leads**.
1. The new filter opens with two ways to start. Under **Build it condition by condition**, press **Add condition**.
2. Pick a field under **Select field**. Then pick how to compare (**is**, **is not**, **is any of**, **contains**…) and the value.
3. Press **Add condition** for more. The **and** between conditions means all of them must match.
4. Press **and** to switch it to **or**: then any one of them may match.
5. From two conditions on, **⋯** under them → **Add a group** puts a group inside the filter. Groups go at most 4 levels down.
6. Once a condition is full, the count ("12 contacts match") and **Matching records** show. They update as you go.
To change the record type, press **Contacts**, **Companies**, **Deals** or **Leads** at the top. With conditions in place it asks first, because a switch clears them.
Also called: advanced search, segment, query builder.

### How do I build a filter by describing it in plain words?
To build a filter from a sentence in Margince, open **Filters and views**, press **New filter** and pick the record type. Type the records you want under **Describe the contacts you want** (or companies, deals, leads). Then press **Propose conditions**.
1. The new conditions show in the builder as rows marked **Proposed**, and the count includes them.
2. Each is an ordinary condition: change its field, way to compare or value and it becomes yours.
3. Above them, **Keep all** makes every proposed row yours. **Replace my conditions** keeps only the proposal, when you had conditions of your own. **Undo** puts the filter back.
4. Anything the fields cannot say, such as "likely to buy", is listed under **Could not use** with the reason.
5. Check the match count, then save. Proposed rows are saved as plain conditions. Nothing is saved until you press **Save**.

Once the filter has conditions, the box folds under **Describe changes in plain words**.

This needs an AI model set up under **Settings** → **AI models**. The AI never gets your CRM records. It gets your sentence, the record type, the date of today, your language, and the fields you can filter on. That means names, types, ways to compare and picklist choices, with the custom field labels you may read. If you can read companies, it also gets what your own company sells and to whom, so it can read words like "our target market".
Also called: natural-language filter, AI filter, describe a segment.

### What can I filter on in Filters and views?
The fields on **Filters and views** in Margince are:
- **Contacts**: **Owner**, **Owner team**, **Tag** and custom fields.
- **Companies** add **Industry**, **Size**, **Lifecycle**, **Relationship type**, **Domain**, and what the company runs (**Mail system**, **Hosting**, **Operated service**, **Technology**).
- **Deals** add **Pipeline**, **Stage**, **Company**, **Partner**, **Project**, **Status**, **Forecast category**, **Company industry**, **Company size** and **Company lifecycle**.
Custom fields have a **Custom field** mark. Your own company is never in a company filter.
Also called: filter fields, which attributes can I filter.

### How do I save a filter from Filters and views?
To save a filter built on **Filters and views**, fill at least one condition and press **Save** at the bottom. **Save this filter** opens.
1. Type a **Name**. Under **Keep it as**, choose **Saved view** (only you can find it) or **Live List** (see [Live Lists and Shortlists](#live-lists-and-shortlists)).
2. Press **Save view** or **Save list**. A saved view opens as its own page, and a Live List opens the list's page.

To use a saved view again, press its name in the library. It opens and shows its conditions as a sentence. Press **Edit conditions**, change them, then press **Save changes**, or **Save as new view** to keep both. If the view changed somewhere else since you opened it, Margince saves nothing and offers **Reload view**. A filter saved here does not show as a tab on the list screens. A list's saved view does not show in the library.

Also called: save a segment, keep an advanced search.

## Selecting several records

### How do I select several records at once?
To select many records in Margince, open **Contacts**, **Companies**, **Leads** or **Deals** as a **Table**. Tick the box at the start of each row. A bar shows above the rows saying "{count} selected", with the actions for that list.
There is no box to tick every row: tick each row. Archived rows have no box, and neither do closed deals or qualified and disqualified leads. **Projects** have no way to tick many rows: change those one at a time.

Also called: multi-select, bulk edit, mass update, select all.

### How do I reassign several contacts, companies or deals to a colleague?
To give many records a new owner at once in Margince, tick them on the **Contacts**, **Companies** or **Deals** table. Choose **Pick an owner** in the bar for many rows, and press **Assign owner**.
1. A window shows how many records will change and up to three examples. It names each record left as it is, with its reason. For example "No permission to change" or "Changed since the list loaded".
2. Press **Change owner**. A message says how many changed and how many were left as they were.
On **Leads**, pick the colleague under **Choose owner** and press **Assign**.
Also called: bulk reassign, hand over accounts, transfer deals, a colleague left, reassign all their contacts.

### What can I do to several records at once?
The bar for many rows on **Contacts**, **Companies** and **Deals** offers **Assign owner**, **Add tag**, **Remove tag**, **Create task**, **Add to Shortlist** and **Archive**. On **Deals** it also offers **Move to stage** (open stages only). On **Leads** it offers **Assign** and **Disqualify** (with a **Reason**).

**Add to Shortlist** shows only when there is a Shortlist of that record type you may change. Every change first shows what it will do; your own company is never archived. A change of more than 10 records shows a "Large change" warning. You cannot win or lose many deals at once. After a change, the message that says how many changed has an **Undo** button. You cannot change custom fields on many records at once.

Also called: bulk actions, mass archive, bulk disqualify.

### Can I tag several records at once?
Yes. To tag many records in Margince, tick them on the **Contacts**, **Companies** or **Deals** table. Choose **Pick a tag** in the bar for many rows, and press **Add tag** or **Remove tag**.
1. A window shows how many records will change, and each record left as it is with its reason. For example "Already has or lacks this tag" or "No permission to change".
2. Press **Add tag** or **Remove tag** to confirm. **Undo** in the message takes the tag off only the records it just tagged.
To work with the tagged records after that, use **Filter** → **Tags** on the list.
Also called: bulk tag, mass tag, tag many contacts, label several deals at once.

### Can I create a task for several records at once?
Yes. Tick the records on the **Contacts**, **Companies** or **Deals** table and press **Create task** in the bar for many rows.
1. Fill in **What has to be done**, and if you want a **Due date** and an **Assignee**. Without an assignee the tasks are yours.
2. Press **Preview**, check the window, and press **Create tasks**. Each record gets its own task.
**Undo** in the message archives the tasks it just created. A task someone has completed or edited since is left as it is.
Also called: bulk task, follow up with many, mass follow-up.

## Live Lists and Shortlists

A list in Margince is a named set of one kind of record (contacts, companies, deals or leads) that you and your colleagues work from. There are two kinds. A **Live List** is a saved filter: its members are whatever matches the filter now. So records join it and leave it on their own as they change. A **Shortlist** is picked by hand: a record is on it because someone put it there, and it stays until someone takes it off.

Lists live on **Filters and views**. A list only you can find is under **Only me**. A shared list is under **Shared**. It shows its kind, record type, how many members you can see and who can find it.

### How do I make a Live List?
To make a Live List in Margince, build its filter on **Filters and views** → **New filter**. Press **Save** and choose **Live List** under **Keep it as**.
1. Pick the record type and add conditions until **Matching records** shows the records you want.
2. Press **Save**, type a **Name**, choose **Live List** and choose **Who can find it**. **Purpose (optional)** says what it is for.
3. Press **Save list**. The list's own page opens.
Also called: dynamic list, smart list, saved segment, audience.

### How do I start a Shortlist?
To start a Shortlist in Margince, open **Filters and views** and press **New Shortlist** next to **New filter**.
1. Type a **Name**, pick the **Record type**, and if you like say **What it is for**. **Who can find it** starts on **A team**; pick **Only me** to keep it to yourself.
2. Press **Create list**.
3. Add records from a record's page: press **Add to Shortlist** and pick the Shortlist (or **A new Shortlist…**).
Add a note under **Why (optional)** and press **Add**. To add many records at once, tick them on the list screen. Pick the Shortlist under **Add to Shortlist** in the bar for many rows.
Also called: static list, hand-picked list, target list, account list.

### Who can find a list?
Each list in Margince says **Who can find it**: **Only me**, **A team** (one of your teams, or all of the owner's teams) or **Everyone**. Change it with **Edit list** under **⋯** on the list's page.
Sharing a list shows nobody a record they could not already see. Each reader sees only the members they may see, and **Records** on the list's page says how many that is ("12 companies"). So two colleagues can see different numbers on the same list. A contact, company, deal or lead page shows the lists that record is on under **Lists**, again only those you can find.

Also called: share a list, list permissions, private list, team list.

### What is a list's steward?
The steward looks after a list. It is whoever made it, unless it was given to someone else. The steward can edit, archive and restore the list and change a Shortlist's members. So can anyone whose role sees every record, such as an admin. Everyone else who can find the list can read it.

When the steward leaves, the library shows **Needs a steward** on the list's row, and the list's page says "Nobody looks after this list". Someone whose role sees every record can press **Look after it** to become its steward.
Also called: list owner, who manages this list.

### Why is this record on the list, or not on it?
A Margince list's page shows why each member is there, in its own columns.
- On a Live List, each field the filter uses is a column with the member's value of today, such as "52 days ago". The first four are shown; **Display** offers the rest. A value you may not see shows as **Hidden**.
- On a Shortlist, **Added by**, **Added on** and **Note** say who picked the record, when, and why.
To see why a record is not on a Live List, open the record's page. Under **Lists**, pick the list in **Check a Live List**. It shows each clause of the filter as **Met**, **Not met** or **No value to judge**, with the record's value of today. A value you may not see shows as "Value hidden from you".

To take one record off a Shortlist, go to **Lists** on the record's page and press **Take off the Shortlist** next to it. It comes off at once. The toast "Taken off {name}" offers **Undo**. **Undo** puts the record back with its **Added by**, **Added on** and **Note**. To take many off, tick them on the list's page and use the bar for many rows.
Also called: why is this contact here, why is this record missing, list membership reason.

### How often does a Live List update?
A Live List's members are worked out again every time you open it, so the list itself is always up to date. Apart from that, Margince checks every Live List every 15 minutes and records who joined and who left. That record is what **What changed** and "since your last visit" show.
- **What changed** on the list's page says **Last checked** with the time of the last check. A new list says "Not checked yet" until its first check.
- The check takes the lists checked longest ago first, so with many lists one can wait longer than 15 minutes.
- A record that joins and leaves between two checks is not recorded.
- A Live List that matches more than 50,000 records is too large to compare. It still shows its members, but **What changed** says "It matched too many records to record who joined and left."
Also called: refresh a list, list sync, when does my list update.

### What changed on a list since my last visit?
Margince keeps a note of when you last opened each list. In the **Filters and views** library, a Live List that changed since then shows "+3 / −1": three joined, one left. On a Live List's page, "Since your visit on" and the date give the same counts. They name the newest three records that joined and that left as links ("+2 more" for the rest), and say how often the filter changed. Each member that joined since your visit has a **New** mark.

**What changed**, at the bottom of the list's page, is the full history. It shows who was added or taken off, and how. That can be by hand, in a change to many rows, by an automation, or because the record was archived or restored. It also shows each change to the list, and **Joined** and **Left** entries by **The 15-minute check**, with the date of its check. You see only the records you may see.
Also called: list history, list activity, who joined, who left, new members.

### How do I act on a list's members?
To change many members of a list at once in Margince, open the list. Tick the members you want, or press **Select all**, and use the bar under **Members**.
1. The bar offers **Assign owner**, **Add tag**, **Remove tag**, **Create task**, **Add to Shortlist** and, except for leads, **Archive**. On a Shortlist you may change, it also offers **Remove from this Shortlist**.
2. Every change shows a preview first: how many records will change and some examples. It names each record left as it is, with its reason. A change of more than 10 records shows a "Large change" warning you confirm.
3. The message after that has an **Undo** button.
One change takes at most 500 records, so **Select all** selects the first 500 and says so. Act on them, then select the rest. **Export CSV** beside the list's name downloads the members you can see. **Exported** on the list's page says how many times the list was exported, and when.
Also called: bulk edit a list, reassign everyone on a list, export a list.

### What does "Uses a retired field" mean on a list?
A Live List's row in the library shows **Uses a retired field** when its filter names a custom field that has been archived. Its page says "This filter uses a retired field". The list still works on the values already stored, but nothing new is recorded in that field. Its steward should change the clause with **Edit filter** on the notice. Before you archive a custom field, Margince names the Live Lists that filter on it.

In the library, **Filter no longer works** means a field the filter names has changed, so the filter cannot run. The list's page says "This filter no longer works". Its steward fixes it the same way, with **Edit filter**.

Also called: broken list, list warning, archived custom field.

### How do I change a Live List's filter?
To change which records a Margince Live List holds, open the list and press **Edit filter**. Or choose **Edit filter** under **⋯** on its row in the library. Its conditions open with a notice that names the list.
1. Change, add or delete conditions; the match count updates as you go. **Discard changes** puts the list's filter back.
2. Press **Save to** and the list's name. The box names any automations that use the list. They act on the new filter from the next check. Press **Save filter**; the list's page opens.
3. From then on records join and leave by the new filter, and **What changed** records the edit.
If someone changed the list after you opened it, Margince says so and saves nothing. Open the list again and make the edit again. To keep a copy instead, choose **Save as new view** under **⋯** at the bottom. Only whoever may change the list (see [What is a list's steward?](#what-is-a-lists-steward)) sees **Edit filter**; a Shortlist has no filter.
Also called: edit a list's criteria, change a segment, update a smart list.

### How do I rename, archive or restore a list?
To change a list in Margince, open it, press **⋯** beside its name and choose **Edit list**. Change the **Name**, **What it is for** and **Who can find it**, then press **Save**. Only whoever may change the list (see [What is a list's steward?](#what-is-a-lists-steward)) sees **⋯**.
- **Archive list**, last under the same **⋯**, makes the list read-only. Its members and history are kept, and **Restore** on the list's page brings it back.
- If an automation uses the list, archiving it names the automations first. They pause, and restoring the list does not start them again.
Also called: delete a list, rename a list, change who can see a list.

### Can an automation act on a Live List?
Yes. **Settings** → **Automations** has three rules that watch a Live List:
<!-- prose:allow bold quotes the rule names as Settings → Automations shows them -->
- **Follow up when a record joins or leaves a Live List**: a task for the record's owner.
<!-- prose:allow bold quotes the rule names as Settings → Automations shows them -->
- **Tell me when a record joins or leaves a Live List**: a notice to you.
<!-- prose:allow bold quotes the rule names as Settings → Automations shows them -->
- **Add to a Shortlist when a record joins or leaves a Live List**.
1. Pick the Live List to watch, and choose whether the rule acts when a record joins, leaves, or either. For the Shortlist rule, pick a Shortlist of the same record type. A record already on it stays as it is. The follow-up task is due in 2 days unless you change it.
2. The rule acts after the 15-minute check, once for each record you can see that joined or left.
3. When one check moves more than 100 records, the rule acts on none of them and pauses itself.

It also pauses when the list is archived, when its filter stops working, or when you can no longer find the list. A paused rule says why and stays paused until its owner starts it again. Restoring or fixing the list does not start it again. A rule cannot start again while its list is archived or its filter is broken. A follow-up task never names the list, because the list may be private.
Also called: list trigger, when someone joins a list, list workflow.

### Why can I not find Lists?
If Margince says "Lists are switched off for this installation.", whoever runs your Margince has turned Lists off in its setup. Nothing is deleted: when Lists are turned back on, every list, member and history entry is there as before. Live Lists are not checked while Lists are off. So the first check after that records at once everyone who joined or left in the time between. Saved views and **Filters and views** keep working all the while.
Also called: lists missing, Live Lists not available.

## Exporting a list

### How do I export a list to CSV or Excel?
To export records from Margince, build a filter on **Filters and views**. Choose **Export CSV** or **Export JSON** under **⋯** at the bottom of the filter. A Live List or Shortlist exports with **Export CSV** beside its name. The **Contacts**, **Companies**, **Deals** and **Leads** screens have no export button.
1. Open **Filters and views**, press **New filter** and pick **Contacts**, **Companies**, **Deals** or **Leads**.
2. Press **Add condition** and fill at least one condition. **⋯** (**More for this filter**) then shows next to **Save**.
3. Choose **Export CSV** (opens in Excel) or **Export JSON**. An opened saved view offers both under **⋯** next to its name.
Only records you can see are exported, and each export goes into the audit log. See [What is kept, what is destroyed](retention-exports-and-deletion.md). To export a Live List or a Shortlist, open it and press **Export CSV** beside its name.
Also called: download a list, export to spreadsheet, extract contacts.
