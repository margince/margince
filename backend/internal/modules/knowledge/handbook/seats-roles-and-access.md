# Seats, roles and who can see what

Margince decides what you can do from three separate things, and they do not
substitute for each other:

1. **Your seat** — full or read-only. A licensing property.
2. **Your role** — what kinds of record you may act on.
3. **Row scope** — which particular records you may act on.

A "no" from any one of them is a no.

## A word on words

**Company** does double duty, and the screen always says which is meant. Your
own company is the tenant — "Everyone in the company", "Whole company",
"Company name". A *company record* is somebody you do business with, listed
under Companies.

**Installation** is the deployment itself — the thing an operator runs. One
installation serves one company.

## Seats

Two kinds.

**Full seat.** Can read and change things, subject to role and row scope.

**Read seat.** Can read, and can never change anything, whatever the role says.
The seat is checked *before* the role, so no combination of permissions gets
around it. The refusal is plain: "This seat is read-only, so the request was
refused. Ask an operator to raise the seat."

**Only full seats are counted against your licence.** Read seats are unlimited
and never metered.

Agents get seats too. An agent seat is always a full seat, and it is **not
counted against your licence**. The licence defines a seat as one identified
human being, and says that automated agents acting under the authority of a
counted seat do not themselves count — so the meter follows the document a
customer actually relies on.

That does not let an installation work without limit through agents, which is
the reading the exclusion invites: an agent may act only where it is
attributable to a counted seat, so an installation with no seats has no
authority for one to act under.

**Your installation does not come with one.** A new installation seeds no agent
identity. What an agent may do comes from the passports colleagues mint for it
and the connections they approve — never from a role of its own, which is why no
agent identity is ever granted one.

### The licence

**Settings → Seats & license** shows **Seats in use** against **Seats granted**. If
nothing caps your seats, it says "No limit" rather than showing a zero.

When you run out, this happens — and this is the important part:

> Nobody loses access and no seat is taken away — but no new member can be
> invited until you are back inside the entitlement. Deactivate a member, or
> raise the entitlement.

No one is demoted. No session ends. Only the *next* seat is refused.

The licence is checked offline at start-up. It is never phoned home.

## The six roles

| Role | Sees | Changes |
|---|---|---|
| **Admin** | every normal record, and all configuration | everything, configuration included |
| **Ops** | every normal record, and all configuration | nearly everything operational — but not members, teams, the audit log, the privacy queue or a reset |
| **Management** | every record in the company | every record; no administrative power |
| **Team Lead** | records | records owned by anyone on a live team with them; configuration read-only; no access to exchange rates, model prices, imports or retention |
| **User** | records | records they own; configuration read-only; full control of their own saved views |
| **Read-only** | every record and most configuration | nothing, except their own saved views |

The sharpest place Admin and Ops differ: **Ops cannot administer members or
teams, cannot read the audit log or the privacy queue, and cannot reset the
installation.** It reads the role directory rather than editing it. Everything
else operational is Ops's.

A member with **no** role sees nothing at all. Every gate fails closed. If
someone reports an empty app, check their role first.

Holding more than one role gives you the widest of them.

The six are seeded with the product, but they are not frozen — an administrator
can change what they grant. Everything on this page describes the **seeded**
grants: what actually decides a request is the permission, so a custom role
holding one reaches what it names, and an Admin whose role lost one does not.

## Row scope: which records, not which kinds

Three levels: **own**, **team**, **all**.

- **own** — records you own, plus records nobody owns
- **team** — those, plus records owned by your teammates
- **all** — everything in the company

Here is the part that differs from most CRMs, and it is deliberate:

> **Reading a contact, company, lead, deal or project ignores row scope
> entirely.**

Every seat holding the read permission reads every contact, company, lead, deal
and project in the company. The app states it: "Reads every contact,
company, lead and deal in the company."

Row scope governs **writes**. Not customer reads. Projects used to be the
exception and are not any more: a consultant delivering a project they neither
owned nor were granted got a 404, which is not a privacy boundary, only a
broken one.

Two things the word "everything" above does NOT cover, for any role including
Admin, because neither is a tier of row scope:

- **Correspondence you were not part of.** Mail and meetings carry their own
  audience, and seniority does not override it.
- **A captured contact still private to the contact whose mailbox made it.** A
  connector creates a contact from a message nothing has judged yet, and it
  belongs to that seat alone until a classifier judges the sender or the owner
  publishes it themselves. An admin gets a 404, which is the point: connecting
  a mailbox with a year of history must not put every correspondent, a lawyer
  and a doctor among them, in front of the company.

The reasoning is that a shared pipeline is the point. Two narrowings survive: a
record created by a connector can stay private to its owner until promoted, and a
per-record share can widen access further.

Your personal things — lists, saved views, automations, your writing voice — keep
the classic owner rule.

**One shipped role is team-scoped: Team Lead.** A Team Lead reads and works the
records of everyone sharing a live team with them, with no share arranged first
— it is the Management grid, bounded to a team instead of the company. A Team
Lead who belongs to no team reaches exactly their own rows.

**User is own-scoped.** For every role but Team Lead, being on a team with
somebody does not by itself let you edit their records; that takes an explicit
share or an unbounded seat.

An **ownerless** record is readable by everyone and writable by nobody until
somebody claims it.

Every record tells the app whether you can write it, so the edit buttons you see
match the answer the server would give. That is a convenience, not the
enforcement — the server checks again regardless.

## Who can see this record

A contact and a company each carry the answer on their own header, in one
place, with the control that changes it:

- **Private to its owner** — "Nobody else in the company can see this account —
  not the team, and not an admin."
- **Shared with the company** — "Everyone in the company can see this contact."

Both directions are ordinary edits, both ways, on both record types. That
matters most for a record capture minted: a company created from a message
nothing has judged yet is owner-only, and before this there was no control to
press — only a sender verdict, which is a machine deciding who may read a
record.

Making a company private again does not reach in and re-hide what is filed
against it: "Deals, contacts and mail already filed against it keep their own
audiences."

Two refusals guard it. A record set to owner-only with **no owner** is refused —
that is a row no seat can read, including the one that would repair it. And
the write is refused if your permission to make it lapsed while you were
deciding, rather than landing on an answer you no longer hold.

Do not confuse this with **Private correspondence** in the sidebar. That is a
capture hold on a mail domain, a different setting with a different effect.

## What you see when you are not allowed

Two different refusals, and the difference is deliberate.

**You may not do this to this kind of record** → a refusal that says so:

> You do not have permission for this action. Ask an admin, or whoever shared
> this record with you, to widen your access.

**You may not see this particular record** → **not found**. Not "forbidden". The
existence of the record is hidden, so a leaked link tells you nothing.

One refinement: a record you can *read* but not *write* gives you the first
refusal, not "not found". It is visibly yours to read, so there is nothing left
to hide.

A member who follows a link to a page their seat does not reach is told so
plainly, and the address is left alone: "This settings page is not yours to
open. The address in the bar is a real page — your seat does not reach it. It is
still there to copy if you need to ask somebody who does".

## Teams

A team is a named group of colleagues. Only an administrator creates one, renames it,
archives it, or changes who is in it. Anyone can see the list of teams.

**A team carries no permissions of its own.** It is not a role. It does two
things:

1. **It is a share target.** You can share a record with a whole team in one act.
   This is its main job today.
2. It resolves team-level row scope, which is what "their team" means for a
   **Team Lead** — the one shipped role that reads it.

Archiving a team stops its memberships resolving anything.

## Sharing one record

You can share a single record with a colleague or a team.

**Five record types can be shared:** contacts, companies, deals, leads and
projects. Configuration cannot.

**Two levels:**

- **Read** — "Can open and read this record — cannot edit or send."
- **Write** — "Can open, edit, and add to this record — not change ownership or
  sharing." Write includes read.

**Expiry:** no expiry, 24 hours, 7 days, or 30 days.

Rules worth knowing:

- **A share is capped at your own access, no wider.** You cannot give away
  something you do not have.
- Someone holding a record through a read share **cannot pass it on**.
- Sharing never widens what a *role* may do — only which records it reaches.
  Share a deal with someone whose role cannot read deals at all and the share
  does nothing.
- A write share to someone on a read seat is refused. Raise the seat first, or
  share read.
- Re-sharing the same record **replaces** the whole share. Anything you leave out
  is cleared, not kept.
- Revoking takes effect on the recipient's next request. There is no undo.
- An agent cannot share at all. The grant verbs reject any non-human caller —
  there is no staged path to it.

There are no sharing hierarchies, no rules-based sharing, and no delegating the
right to share. Flat, explicit grants only. Every share and un-share is audited.

## Inviting and removing colleagues

Only an administrator can, and only a human — an agent may never create a human
account.

Inviting and deactivating answer to the `user_admin` permission, so a custom role
holding it manages colleagues without being called Admin.

**One ceiling stands above that permission.** Acting on an **Admin's** account —
inviting one, changing their role, deactivating them — takes the literal Admin
role, whatever else you hold. A grant that could reach an Admin would be a way to
become one.

**Inviting.** You choose a role, and the colleague is created with no password. If
your installation sends email, they get a link. If not, the administrator mints a
one-time link and hands it over directly: "Send this link to the member over a
channel you trust. It works once and is shown only now." The link lasts 7 days.

Before you send it, you can **preview what this contact will see**, computed from
the same permission data the real gates read — so the invite screen shows the
truth rather than a second interpretation of it.

An invite is refused if you are out of seats. Nothing about that clears on its
own, so there is no point retrying.

**Changing a role replaces it.** "Holds {roles}. Choosing one replaces them all."

**Deactivating** signs them out everywhere and revokes their agent passports
immediately, in one step. You can reactivate them later, but they will need to
sign in again. Reactivating takes a seat back, so it can be refused if you are
full — a read seat never is.

The last active administrator cannot be deactivated or demoted.

Deactivating an **agent identity** stops what that agent was doing on your data.
It does not stop scheduled extension jobs: a tick acts as the job, not as an
identity, and each record it captures is landed under the live authority of the
member whose connection produced it. To stop one of those, disconnect the member
or retire the unit.

## Field masking

The product can hide a single column of a record you are otherwise allowed to
read. Nothing uses it today. There was once a rule hiding deal values from
members, and it was removed — **deal values are visible to everyone who can read
the deal.**

If your administrator ever does set a mask, the value reads as empty and the
record names which fields were withheld. Sorting or filtering by a hidden column
is refused rather than quietly returning wrong results.

## Where the audit trail fits

Everything above decides what a seat *can* do. The audit trail records what it
*did*: every action, attributed to a human, an agent or a connector, with the
authorization rule that allowed it.

Only an administrator reads it, because it names every actor and every record
they touched. See
[What is kept, what is destroyed](retention-exports-and-deletion.md#the-audit-trail).
