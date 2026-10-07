<!-- prose:plain -->
# Seats, roles and who can see what

What you can do in Margince is decided by three things, and one cannot stand in for another:

1. **Your seat**: full or read-only. It comes from the licence.
2. **Your role**: what kinds of record you may act on.
3. **Row scope**: which records you may act on.

A "no" from any one of them is a no. **Settings → Overview** shows your own answer to all three under **Your seat**, **Your role** and **Visible records**.

## Managing colleagues, roles and teams

### How do I invite a colleague to Margince?
To invite a colleague to Margince, open **Settings → Members** and choose **Invite user**.
1. Open the account menu at the top right and choose **Settings**, then **Members**.
2. Choose **Invite user**.
3. Fill **Email** (required) and **Full name** (required).
4. Pick a **Role** and tick any **Teams** they should join.
5. Check the **User access** preview, then choose **Invite**.
You need the permission to manage users. Without it the page says "Your role cannot manage users." An invite is refused when no seat is left.
Also called: add a user, add a teammate, create an account, onboard a colleague.

### How do I change someone's role or make someone an admin?
To change a colleague's role in Margince, open **Settings → Members** and pick the new role in the role menu on their row.
1. Open **Settings → Members**.
2. Find the colleague in the **Users** list.
3. Open the role menu on their row (it reads **Set role…** when they hold no role). Pick **Admin**, **Management**, **Team lead**, **User**, **Read-only** or **Ops**.
4. The note "Role changed for {name}" confirms it.
Choosing a role takes the place of every role they held. Only an Admin can give or take away the Admin role.
Also called: promote, demote, change permissions, make admin.

### How do I remove a user from Margince?
To remove a colleague from Margince, open **Settings → Members** and choose **Deactivate** on their row.
1. Open **Settings → Members**.
2. Open the **Actions for {name}** menu on their row.
3. Choose **Deactivate**, then confirm **Deactivate** in the **Deactivate {name}?** box.
They are signed out everywhere and their agent passports end at once. Margince does not delete user accounts. A user you deactivate stays in the list and frees their seat. You cannot deactivate the last active admin.
Also called: delete a user, offboard, remove a teammate, disable an account.

### How do I reactivate a deactivated user?
To bring back a colleague you deactivated, open **Settings → Members**. Open the **Actions for {name}** menu on their row and choose **Reactivate**.
They must sign in again after that. Bringing them back takes a full seat again, so it is refused when the licence is full.
Also called: restore a user, re-enable an account.

### A colleague left: who gets their deals, and how do I reassign their records?
When a colleague leaves and you deactivate them in Margince, they stay the owner of their deals, leads, contacts and companies. Nothing gets a new owner on its own. Give the work to others before you deactivate them.
1. List what they own: **Filters and views** → **Add clause** → **Owner**.
2. **Deals** and **Leads**: tick the rows and press **Assign** in the bar for many rows. See [Lists, filters and views](lists-filters-and-views.md).
3. **Contacts**, **Companies** and **Projects**, one at a time: **Owner** in **Details**.
On a project, use **More actions** → **Assign to a colleague**.
Also called: offboarding, handover, a rep left, transfer a colleague's accounts.

### How do I reset someone's password?
To reset a colleague's password in Margince, open **Settings → Members**. Open the **Actions for {name}** menu on their row and choose **Get set-password link**.
1. Choose **Get set-password link**.
2. Choose **Copy link** in the **Set-password link for {name}** box.
3. Send it to them yourself. The box says: "Send this link to the member through a trusted channel. It works once and is shown only now."
The box shows when the link ends. To change your own password, use **Settings → Account → Change password** instead.
Also called: forgot password, password reset, locked out.

### How do I create a team?
To create a team in Margince, open **Settings → Teams** and choose **New team**.
1. Open **Settings → Teams**.
2. Choose **New team**.
3. Fill **Team name** (required), for example "DACH Sales".
4. Choose **Create team**.
To add colleagues, an Admin opens the team and ticks their names under **Team members**. You can add only active human users. Without the permission the page says "Your role cannot manage teams."
Also called: group, squad, sales team.

### How do I change someone's seat from read to full?
Margince has no screen to switch a seat between full and read-only. Every invite creates a full seat, and **Settings → Overview** shows which seat you hold. To change a seat, ask whoever runs your Margince. A read seat that tries to change something is told: "This seat is read-only, so the request was refused. Ask an administrator to upgrade the seat."
Also called: upgrade a seat, viewer licence.

## A word on words

The word **company** is used for two things in Margince, and the screen always says which one it means. Your own company is the one that uses Margince: "Everyone in the company", "Whole company", "Company name". A *company record* is a business you work with, listed under Companies.

An **installation** is one running copy of Margince, looked after by whoever runs it. One installation is for one company.

## Seats: full seat and read seat

Margince has two kinds of seat, a full seat and a read seat.

**A full seat** can read and change things, but only as its role and row scope allow.

**A read seat** can read, and can never change anything, whatever the role says. The seat is checked *before* the role, so no mix of permissions gets past it. The refusal is plain: "This seat is read-only, so the request was refused. Ask an administrator to upgrade the seat." A write share to a read seat is refused the same way.

**Only full seats are counted against your licence.** You can have as many read seats as you like, and they are never counted.

Agents get seats too. An agent seat is always a full seat, and it is **not counted against your licence**. The licence says a seat is one named human, and agents that act for a counted seat do not count.

That does not let a Margince do work without limit through agents. An agent may act only where a counted seat stands behind it. So a Margince with no seats has nobody for an agent to act for.

**A new installation starts with no agent.** What an agent may do comes from the passports colleagues make for it and the connections they approve, never from a role of its own. See [Agents, passports and what they may do](agents-and-passports.md).

### The licence

**Settings → Seats and license** shows the **Seats** in use as "{used} of {granted}", or "No limit" if nothing limits them.

When more seats are in use than the licence allows, Margince says:

> No one loses access and no seat is removed, but no new member can be invited until the count is within the entitlement. Deactivate a member or raise the entitlement.

Nobody loses a role. Nobody is signed out. Only the *next* seat is refused.

Margince checks the licence itself when it starts. It never calls out or reports back to anyone.

## The six roles

Margince comes with six roles: Admin, Ops, Management, Team lead, User and Read-only.

| Role | Sees | Changes |
|---|---|---|
| **Admin** | every normal record, and all settings | everything, settings included |
| **Ops** | every normal record, and all settings | most of the daily work, but not members, teams, the audit log, the privacy list or a reset |
| **Management** | every record in the company | every record; no admin rights |
| **Team lead** | records | records owned by anyone on a live team with them; settings read-only; no access to exchange rates, AI prices, imports or retention |
| **User** | records | records they own; settings read-only; all of their own saved views |
| **Read-only** | every record and most settings | nothing, except their own saved views |

This is the biggest difference between Admin and Ops: **Ops cannot manage members or teams.** It also cannot read the audit log or the privacy list, or reset the installation. It reads the list of roles but does not edit it. Everything else in daily running belongs to Ops.

A member with **no** role sees nothing at all. Every check starts from no. If someone says the app is empty, check their role first.

If you hold more than one role, you get the most that any of them allows.

You can copy and change roles in **Settings → Roles and permissions**. A new role starts as a copy of another (**New role**, then **Copy rights from**). Only an Admin copies, restores or widens a role; other users who may edit roles rename, narrow or archive one. An archived role gives nothing. The table above shows the six roles as they come. A changed or custom role reaches what its permissions allow.

## Row scope: which records, not which kinds

Row scope has three levels: **own**, **team**, **all**.

- **own**: records you own, plus records nobody owns
- **team**: those, plus records owned by your teammates
- **all**: everything in the company

This is not how most CRM products work. Any seat that can read records reads every contact, company, lead, deal and project in the company, whatever its row scope. The app says so: "Reads every contact, company, lead and deal in the company." Row scope decides which records you may change. This includes projects.

Two exceptions count for every role, Admin included:

- **Mail and meetings you were not part of.** Mail and meetings have their own
  list of who may see them, and no role changes that.
- **A captured contact still private to its owner.** A connection creates a
  contact from a message that nothing has judged yet. The contact belongs to the
  colleague whose mailbox made it, until Margince judges the sender or the
  owner makes it public. An admin cannot open it either. So when you connect a mailbox with a year of history, the whole company does not see everyone the user has written to. That may include a lawyer or a doctor.

A shared pipeline is the default. A share on one record can widen access further.

Saved views, automations and your writing voice keep the owner rule; a shared list shows each reader only the records they may see.

**Only Team lead has team scope.** A Team lead reads and works on the records of everyone who shares a live team with them, with no share set up first. A Team lead who belongs to no team reaches only their own records.

**User has the scope own.** For every role but Team lead, being on a team with someone does not by itself let you edit their records. That takes a share made for it, or a seat with no row limit.

Everyone can read a record with **no owner**, and nobody can write to it until someone claims it.

## Seeing and sharing records

### Why can't I see a record?
When you cannot see a record in Margince, one of three things is true. The record is private to its owner, it is mail you were not part of, or the link names a record that does not exist. Margince answers all three with **not found**, so a link alone shows nothing.

Ask the record's owner to share it with you (**Share** on the record). Or ask them to choose **All users in the company** behind the access chip under the record's name. An admin's role does not open a private record either: only a share does. A record you can read but not edit gives a different refusal: "You do not have permission for this action. Ask an administrator, or the user who shared this record, to extend your access."
Also called: record missing, access denied, 404, cannot open a contact.

### How do I share a record with a colleague?
To share one contact, company, lead, deal or project with a colleague or a team, open the record and choose **Share**. Then fill in **Share this record**.
1. Open the record and choose **Share** in its header.
2. Under **Grant access**, pick the colleague or team.
3. Choose the **Access level**: **Read** or **Write**.
4. Choose an **Expiry**: **No expiry (until revoked)**, **Expires in 24 hours**, **Expires in 7 days** or **Expires in 30 days**.
5. If you like, add a **Reason**, then choose **Grant access**.
You cannot share more access than you hold.
Also called: give access, grant access, collaborate on a deal.

### How do I stop sharing a record?
To take back a share in Margince, open the record and choose **Share**. Find the colleague or team under **Shared with** and choose **Revoke**. The question says: "Revoke this access? It ends at the next request and cannot be undone." To lower write to read instead, share again with **Read**; Margince asks **Reduce access?** first.
Also called: unshare, remove access.

### How do I make a contact or company visible to everyone?
To make a private contact or company open to the whole company, open it and press the access chip under its name (it reads **Private**). Choose **All users in the company** and press **Save**. Nothing changes until you press **Save**, so you can move through the choices with the keys and nothing happens.

To hide it again, press the chip (it now reads **Shared**), choose **Only the owner** and press **Save**. Users and teams it was shared with keep access. **Manage access** in the same panel opens the full list of who can open the record and why. Only someone who may change the record sees the choice; anyone else, and anyone on an archived record, sees the reason instead.

A record with **no owner** cannot be set to its owner only, because no seat could then read it. On such a record **Only the owner** is greyed out, with the note "Assign an owner first". If you are not the owner, **Only the owner** warns "You lose access unless it is shared with you or your team". If no share reaches you, Margince then takes you back to the list.
Also called: publish a contact, make public, make private.

## Who can see this record

A contact and a company each show the answer under their own name, as one chip that reads **Shared** or **Private**. Pressing it opens one sentence that says who can see the record, the choice that changes it, and **Manage access**:

- **Private**, read by its owner: "Only you and the users or teams it was shared with can see this contact."
- **Private**, read by anyone else who can open it, naming the owner: "Private to Mira Voss. You can see this contact because it was shared with you or your team." A private record reaches nobody else, an admin included; a share is the only way in.
- **Shared**: "All users in the company can see this contact."

Both ways are ordinary edits on both record types. This helps most with a record that capture created. A company created from a message that nothing has judged yet is set to its owner only. This choice lets its owner make it public, so they do not wait for Margince to decide.

Making a company private again does not hide again what is filed on it: "This company is now private to its owner. Deals, contacts and mail filed against it keep their own visibility." Making a contact private keeps access for users and teams it was shared with.

The change is also refused if your permission to make it ended while you were deciding. It does not go through on a right you no longer hold.

This is not the same as a capture hold on a mail domain, which is a different setting that does something else.

## What you see when you are not allowed

Margince gives two different refusals.

*You may not do this to this kind of record* → a refusal that says so:

> You do not have permission for this action. Ask an administrator, or the user
> who shared this record, to extend your access.

*You may not see this record* → **not found**, so you cannot tell the record is there, and a link alone tells you nothing.

One more point: a record you can *read* but not *write* gives you the first refusal, not "not found". You can plainly read it, so there is nothing left to hide.

A member may follow a link to a settings page their role does not reach. They see **No access to this settings page**:

> This page exists, but your role cannot open it. Copy the address to ask someone who has access.

## Teams

A team in Margince is a named group of colleagues. To create or rename one you need the permission to manage teams. Only an Admin changes who is on a team, or archives or restores one. Anyone can see the teams.

**A team has no permissions of its own.** It does two things:

1. **You can share with it.** You can share a record with a whole team in one step.
2. It gives the meaning of team row scope: what "their team" means for a **Team lead**. That is the one role Margince comes with that reads it.

The Teams page says it: "For most roles, membership grants no access. A team lead added to a team can read and edit that team’s records without a share."

When a team is archived, being in it gives nothing any more. You can restore an archived team.

## Sharing one record

A share in Margince gives one colleague or one team access to a single record.

**Five record types can be shared:** contacts, companies, deals, leads and projects. Settings cannot.

**Two levels:**

- **Read**: "Can open and read this record, but not edit or send."
- **Write**: "Can open, edit and add to this record, but not change ownership or
  sharing." Write includes read.

**Expiry:** none, 24 hours, 7 days, or 30 days.

Rules worth knowing:

- **A share cannot go past your own access.** You cannot give away something you
  do not have.
- Someone who holds a record through a read share **cannot pass it on**.
- Sharing never widens what a *role* may do, only which records it reaches.
  Share a deal with someone whose role cannot read deals at all and the share
  does nothing.
- A write share to someone on a read seat is refused: "A read-only seat cannot
  hold write access. Upgrade the seat first, or grant read access."
- Sharing the same record again **replaces** the whole share. Anything you leave out
  is cleared.
- Taking back a share counts from the next request of the user it was shared with. There is no undo.
- An agent cannot share at all, and cannot ask a colleague to approve a share
  for it.

There are no levels of shares, no sharing by rules, and no way to give someone else the right to share. Only plain shares, each one made by hand. Every share and every end of a share goes into the audit log.

## Inviting and removing colleagues: the rules

To invite and remove colleagues you need the permission to manage users. Only the Admin role holds it by default, and only a human may use it: an agent may never create an account for a human.

Inviting, deactivating and the **Members** settings list all answer to that permission. So a custom role that holds it manages colleagues without being called Admin.

**Your access must cover theirs.** To act on an **Admin's** account you need the Admin role yourself. If not, deactivating or bringing them back needs their admin permissions and row scope. A set-password link, a role change or an invite needs everything the account holds or will hold, fields and teams included. Only an Admin changes their own role. A member on an archived role gets no link and cannot be brought back until an Admin gives them a live role.

**Inviting.** You choose a role, and the colleague is created with no password. If your Margince sends email, they get a link. If not, the admin takes a link that works once from **Get set-password link** and gives it to them. An invite link lasts 7 days.

Before you send the invite, the **User access** panel shows what the new colleague will reach.

An invite is refused if you are out of seats. Trying again does not help until a seat is freed or the licence is raised.

**Changing a role replaces it.** "Holds {roles}. Choosing a role replaces all of them."

**Deactivating** signs them out everywhere and ends their agent passports in one step. You can bring them back later, and they must then sign in again. Bringing them back takes a seat again, so it can be refused if you are full; for a read seat it never is. Their records keep them as owner until someone gives the records to someone else.

You cannot deactivate the last active admin, or take the Admin role from them.

## Field masking

Everyone who can read a deal sees its value. Field masking can hide a single field of a record you may read in every other way, but nothing hides a field by default.

If a field is masked, it reads as empty and the record names which fields were held back. Sorting or filtering by a masked field is refused, so the list never shows results that are wrong without telling you.

## Where the audit trail fits

Seats, roles and row scope decide what a seat *can* do. The audit trail records what it *did*: every action, with the human, agent or connection behind it, and the rule that allowed it.

To read it you need the permission to read the audit log. Only the Admin role holds it by default, because it names everyone who acted and every record they opened. See [What is kept, what is destroyed](retention-exports-and-deletion.md#the-audit-trail).
