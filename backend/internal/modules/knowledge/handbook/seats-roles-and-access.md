# Seats, roles and who can see what

What you can do in Margince is decided by three separate things, and they do not
substitute for each other:

1. **Your seat**: full or read-only. A licensing property.
2. **Your role**: what kinds of record you may act on.
3. **Row scope**: which particular records you may act on.

A "no" from any one of them is a no. **Settings → Overview** shows your own
answer to all three under **Your seat**, **Your role** and **Visible records**.

## Managing colleagues, roles and teams

### How do I invite a colleague to Margince?
To invite a colleague or teammate to Margince, open **Settings → Members** and choose **Invite user**.
1. Open the account menu at the top right and choose **Settings**, then **Members**.
2. Choose **Invite user**.
3. Fill **Email** (required) and **Full name** (required).
4. Pick a **Role** and tick any **Teams** they should join.
5. Check the **User access** preview, then choose **Invite**.
You need the user-admin permission; without it the page says "Your role cannot manage users." An invite is refused when no seat is left.
Also called: add a user, add a teammate, create an account, onboard a colleague.

### How do I change someone's role or make someone an admin?
To change a colleague's role in Margince, open **Settings → Members** and pick the new role in the role menu on their row.
1. Open **Settings → Members**.
2. Find the colleague in the **Users** list.
3. Open the role menu on their row (it reads **Set role…** when they hold no role) and pick **Admin**, **Management**, **Team lead**, **User**, **Read-only** or **Ops**.
4. The toast "Role changed for {name}" confirms it.
Choosing a role replaces every role they held. Only an Admin can give or take away the Admin role.
Also called: promote, demote, change permissions, make admin.

### How do I remove a user from Margince?
To remove a colleague from Margince, open **Settings → Members** and choose **Deactivate** on their row.
1. Open **Settings → Members**.
2. Open the **Actions for {name}** menu on their row.
3. Choose **Deactivate**, then confirm **Deactivate** in the **Deactivate {name}?** dialog.
They are signed out everywhere and their agent passports are revoked immediately. Margince does not delete user accounts; a deactivated user stays in the list and frees their seat. The last active administrator cannot be deactivated.
Also called: delete a user, offboard, remove a teammate, disable an account.

### How do I reactivate a deactivated user?
To bring back a deactivated colleague, open **Settings → Members**, open the **Actions for {name}** menu on their row and choose **Reactivate**.
They must sign in again afterwards. Reactivating takes a full seat back, so it is refused when the licence is full.
Also called: restore a user, re-enable an account.

### A colleague left: who gets their deals, and how do I reassign their records?
When a colleague leaves and is deactivated in Margince, their deals, leads, contacts and companies stay owned by them: nothing is reassigned automatically. Hand the work over before you deactivate them.
1. List what they own: **Filters and views** → **Add clause** → **Owner**.
2. **Deals** and **Leads**: tick the rows on the table and press **Assign** in the bulk bar; see [Lists, filters and views](lists-filters-and-views.md).
3. **Contacts** and **Companies**: change **Owner** in each record's **Details** panel.
Also called: offboarding, handover, a rep left, transfer a colleague's accounts.

### How do I reset someone's password?
To reset a colleague's password in Margince, open **Settings → Members**, open the **Actions for {name}** menu on their row and choose **Get set-password link**.
1. Choose **Get set-password link**.
2. Choose **Copy link** in the **Set-password link for {name}** dialog.
3. Send it to them yourself. The dialog says: "Send this link to the member through a trusted channel. It works once and is shown only now."
The dialog shows when the link expires. To change your own password, use **Settings → Account → Change password** instead.
Also called: forgot password, password reset, locked out.

### How do I create a team?
To create a team in Margince, open **Settings → Teams** and choose **New team**.
1. Open **Settings → Teams**.
2. Choose **New team**.
3. Fill **Team name** (required), for example "DACH Sales".
4. Choose **Create team**.
To add colleagues, open the team and tick their names under **Team members**. Only active human users can be added. Without the permission the page says "Your role cannot manage teams."
Also called: group, squad, sales team.

### How do I change someone's seat from read to full?
Margince has no screen for switching a seat between full and read-only. Every invitation creates a full seat, and **Settings → Overview** shows which seat you hold. To change a seat, ask whoever runs your installation. A read seat that tries to change something is told: "This seat is read-only, so the request was refused. Ask an administrator to upgrade the seat."
Also called: upgrade a seat, viewer licence.

## A word on words

The word **company** does double duty in Margince, and the screen always says
which is meant. Your own company is the tenant: "Everyone in the company",
"Whole company", "Company name". A *company record* is a business you deal
with, listed under Companies.

An **installation** is the deployment itself, the thing an operator runs. One
installation serves one company.

## Seats: full seat and read seat

Margince has two kinds of seat, a full seat and a read seat.

**A full seat** can read and change things, subject to role and row scope.

**A read seat** can read, and can never change anything, whatever the role says.
The seat is checked *before* the role, so no combination of permissions gets
around it. The refusal is plain: "This seat is read-only, so the request was
refused. Ask an administrator to upgrade the seat." A write share to a read seat
is refused the same way.

**Only full seats are counted against your licence.** Read seats are unlimited
and never metered.

Agents get seats too. An agent seat is always a full seat, and it is **not
counted against your licence**: the licence defines a seat as one identified
human being, and agents acting under a counted seat's authority do not count.

That does not let an installation work without limit through agents: an agent
may act only where it is attributable to a counted seat, so an installation with
no seats has no authority for one to act under.

**A new installation seeds no agent identity.** What an agent may do comes from
the passports colleagues mint for it and the connections they approve, never
from a role of its own. See [Agents, passports and what they may do](agents-and-passports.md).

### The licence

**Settings → Seats and license** shows the **Seats** in use as "{used} of
{granted}", or "No limit" if nothing caps them.

When seats in use exceed the licence, Margince says: "No one loses access and no
seat is removed, but no new member can be invited until the count is within the
entitlement. Deactivate a member or raise the entitlement."

No one is demoted. No session ends. Only the *next* seat is refused.

The licence is checked offline at start-up. It is never phoned home.

## The six roles

Margince ships six roles: Admin, Ops, Management, Team lead, User and Read-only.

| Role | Sees | Changes |
|---|---|---|
| **Admin** | every normal record, and all configuration | everything, configuration included |
| **Ops** | every normal record, and all configuration | nearly everything operational, but not members, teams, the audit log, the privacy queue or a reset |
| **Management** | every record in the company | every record; no administrative power |
| **Team lead** | records | records owned by anyone on a live team with them; configuration read-only; no access to exchange rates, model prices, imports or retention |
| **User** | records | records they own; configuration read-only; full control of their own saved views |
| **Read-only** | every record and most configuration | nothing, except their own saved views |

The sharpest difference between Admin and Ops: **Ops cannot administer members
or teams, cannot read the audit log or the privacy queue, and cannot reset the
installation.** It reads the role directory rather than editing it. Everything
else operational is Ops's.

A member with **no** role sees nothing at all. Every gate fails closed. If
someone reports an empty app, check their role first.

Holding more than one role gives you the widest of them.

The six roles are seeded with the product, but they are not frozen: an
administrator can change what they grant. This page describes the **seeded**
grants. What actually decides a request is the permission, so a custom role
holding one reaches what it names, and an Admin whose role lost one does not.

## Row scope: which records, not which kinds

Row scope has three levels: **own**, **team**, **all**.

- **own**: records you own, plus records nobody owns
- **team**: those, plus records owned by your teammates
- **all**: everything in the company

Here is the part that differs from most CRMs, and it is deliberate:

> **Reading a contact, company, lead, deal or project ignores row scope
> entirely.**

Every seat holding the read permission reads every contact, company, lead, deal
and project in the company. The app states it: "Reads every contact, company,
lead and deal in the company."

Row scope governs **writes**, not customer reads. Projects used to be the
exception and are not any more: a consultant delivering a project they neither
owned nor were granted got a 404, which is not a privacy boundary, only a
broken one.

Two things "everything" above does NOT cover, for any role including Admin,
because neither is a tier of row scope:

- **Correspondence you were not part of.** Mail and meetings carry their own
  audience, and seniority does not override it.
- **A captured contact still private to the colleague whose mailbox made it.** A
  connector creates a contact from a message nothing has judged yet, and it
  belongs to that seat alone until a classifier judges the sender or the owner
  publishes it themselves. An admin gets a 404, which is the point: connecting
  a mailbox with a year of history must not put every correspondent, a lawyer
  and a doctor among them, in front of the company.

The reasoning is that a shared pipeline is the point. Two narrowings survive: a
record created by a connector can stay private to its owner until promoted, and a
per-record share can widen access further.

Your personal things (lists, saved views, automations, your writing voice) keep
the classic owner rule.

**Team lead is the one team-scoped role.** A Team lead reads and works the
records of everyone sharing a live team with them, with no share arranged first.
A Team lead who belongs to no team reaches exactly their own rows.

**User is own-scoped.** For every role but Team lead, being on a team with
somebody does not by itself let you edit their records; that takes an explicit
share or an unbounded seat.

An **ownerless** record is readable by everyone and writable by nobody until
somebody claims it.

Every record tells the app whether you can write it, so the edit buttons you see
match the answer the server would give. That is a convenience, not the
enforcement: the server checks again regardless.

## Seeing and sharing records

### Why can't I see a record?
When you cannot see a record in Margince, one of three things is true: the record is private to its owner, it is correspondence you were not part of, or the link names a record that does not exist. Margince answers all three with **not found**, so a leaked link reveals nothing.
Ask the record's owner to share it with you (**Share** on the record) or to choose **Share with all users**. A record you can read but not edit gives a different refusal: "You do not have permission for this action. Ask an administrator, or the user who shared this record, to extend your access."
Also called: record missing, access denied, 404, cannot open a contact.

### How do I share a record with a colleague?
To share one contact, company, lead, deal or project with a colleague or a team, open the record and choose **Share**, then fill in **Share this record**.
1. Open the record and choose **Share** in its header.
2. Under **Grant access**, pick the colleague or team.
3. Choose the **Access level**: **Read** or **Write**.
4. Choose an **Expiry**: **No expiry (until revoked)**, **Expires in 24 hours**, **Expires in 7 days** or **Expires in 30 days**.
5. Optionally add a **Reason**, then choose **Grant access**.
You cannot share more access than you hold.
Also called: give access, grant access, collaborate on a deal.

### How do I stop sharing a record?
To take back a share in Margince, open the record, choose **Share**, find the colleague or team under **Shared with** and choose **Revoke**. The confirmation says: "Revoke this access? It ends at the next request and cannot be undone." To reduce write to read instead, grant again with **Read**; Margince asks **Reduce access?** first.
Also called: unshare, remove access.

### How do I make a contact or company visible to everyone?
To make a private contact or company visible to the whole company, open it and use **Who can see this contact** (or **Who can see this company**) in its header, then choose **Share with all users**. To hide it again, choose **Make private**.
A record set to owner-only with **no owner** is refused, because no seat could then read it.
Also called: publish a contact, make public, make private.

## Who can see this record

A contact and a company each carry the answer on their own header, in one
place, with the control that changes it:

- **Private to its owner**: "Private to its owner. No one else in the company
  can see this contact, including team members and administrators."
- **Shared with the company**: "All users in the company can see this contact."

Both directions are ordinary edits on both record types. That matters most for
a record capture created: a company created from a message nothing has judged
yet is owner-only, and this control lets its owner publish it rather than
waiting for a machine to decide.

Making a company private again does not re-hide what is filed against it: "This
company is now private to its owner. Deals, contacts and mail filed against it
keep their own visibility." Making a contact private keeps access for users it
was shared with directly.

The write is also refused if your permission to make it lapsed while you were
deciding, rather than landing on an answer you no longer hold.

This is not the same as a capture hold on a mail domain, which is a different
setting with a different effect.

## What you see when you are not allowed

Margince gives two different refusals, and the difference is deliberate.

**You may not do this to this kind of record** → a refusal that says so:

> You do not have permission for this action. Ask an administrator, or the user
> who shared this record, to extend your access.

**You may not see this particular record** → **not found**. Not "forbidden". The
existence of the record is hidden, so a leaked link tells you nothing.

One refinement: a record you can *read* but not *write* gives you the first
refusal, not "not found". It is visibly yours to read, so there is nothing left
to hide.

A member who follows a link to a settings page their role does not reach sees
**No access to this settings page**: "This page exists, but your role cannot
open it. Copy the address to ask someone who has access."

## Teams

A team in Margince is a named group of colleagues. Creating one, archiving
it or changing who is in it takes the team-administration permission
(`team_admin`), which only the Admin role holds by default. Anyone can see the
list of teams.

**A team carries no permissions of its own.** It is not a role. It does two
things:

1. **It is a share target.** You can share a record with a whole team in one act.
2. It resolves team-level row scope, which is what "their team" means for a
   **Team lead**, the one shipped role that reads it.

The Teams page says it: "For most roles, membership grants no access. A team
lead added to a team can read and edit that team’s records without a share."

Archiving a team stops its memberships resolving anything. An archived team can
be restored.

## Sharing one record

A share in Margince gives one colleague or one team access to a single record.

**Five record types can be shared:** contacts, companies, deals, leads and
projects. Configuration cannot.

**Two levels:**

- **Read**: "Can open and read this record, but not edit or send."
- **Write**: "Can open, edit and add to this record, but not change ownership or
  sharing." Write includes read.

**Expiry:** no expiry, 24 hours, 7 days, or 30 days.

Rules worth knowing:

- **A share is capped at your own access, no wider.** You cannot give away
  something you do not have.
- Someone holding a record through a read share **cannot pass it on**.
- Sharing never widens what a *role* may do, only which records it reaches.
  Share a deal with someone whose role cannot read deals at all and the share
  does nothing.
- A write share to someone on a read seat is refused: "A read-only seat cannot
  hold write access. Upgrade the seat first, or grant read access."
- Re-sharing the same record **replaces** the whole share. Anything you leave out
  is cleared, not kept.
- Revoking takes effect on the recipient's next request. There is no undo.
- An agent cannot share at all. The grant verbs reject any non-human caller;
  there is no staged path to it.

There are no sharing hierarchies, no rules-based sharing, and no delegating the
right to share. Flat, explicit grants only. Every share and un-share is audited.

## Inviting and removing colleagues: the rules

Inviting and removing colleagues takes the user-administration permission
(`user_admin`), which only the Admin role holds by default, and only a human may
use it: an agent may never create a human account.

Inviting, deactivating and the **Members** settings list all answer to that
permission, so a custom role holding it manages colleagues without being called
Admin.

**One ceiling stands above that permission.** Acting on an **Admin's** account
(inviting one, changing their role, deactivating them) takes the literal Admin
role, whatever else you hold. A grant that could reach an Admin would be a way to
become one.

**Inviting.** You choose a role, and the colleague is created with no password.
If your installation sends email, they get a link. If not, the administrator
takes a one-time link from **Get set-password link** and hands it over directly.
An invitation link lasts 7 days.

Before you send the invitation, the **User access** panel previews what the new
user will be able to reach, computed from the same permission data the real
gates read, so the invite screen shows the truth rather than a second
interpretation of it.

An invite is refused if you are out of seats. Nothing about that clears on its
own, so there is no point retrying.

**Changing a role replaces it.** "Holds {roles}. Choosing a role replaces all of
them."

**Deactivating** signs them out everywhere and revokes their agent passports
in one step; they can be reactivated later and must then sign in again.
Reactivating takes a seat back, so it can be refused if you are full; a read
seat never is. Their records keep them as owner until someone reassigns them.

The last active administrator cannot be deactivated or demoted.

Deactivating the company's **agent identity** stops what that agent was doing on
your data. It does not stop scheduled extension jobs: a job acts as itself, and
each record it captures is landed under the live authority of the member whose
connection produced it. To stop one of those, disconnect the member or retire
the extension.

## Field masking

Field masking in Margince can hide a single column of a record you are otherwise
allowed to read. Nothing uses it today. There was once a rule hiding deal values
from members, and it was removed: **deal values are visible to everyone who can
read the deal.**

If your administrator ever does set a mask, the value reads as empty and the
record names which fields were withheld. Sorting or filtering by a hidden column
is refused rather than quietly returning wrong results.

## Where the audit trail fits

Seats, roles and row scope decide what a seat *can* do. The audit trail records
what it *did*: every action, attributed to a human, an agent or a connector,
with the authorization rule that allowed it.

Reading it takes the audit-log read permission (`audit_log`), which only the
Admin role holds by default, because it names every actor and every record they
touched. See
[What is kept, what is destroyed](retention-exports-and-deletion.md#the-audit-trail).
