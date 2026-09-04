# Seats, roles and who can see what

Margince decides what you can do from three separate things, and they do not
substitute for each other:

1. **Your seat** — full or read-only. A licensing property.
2. **Your role** — what kinds of record you may act on.
3. **Row scope** — which particular records you may act on.

A "no" from any one of them is a no.

## A word on words

The product's noun for your tenant is **organization**. It is what every screen
says: "Everyone in the organization", "Whole organization", "Organization name".

Two neighbouring words mean different things:

- **Installation** is the deployment itself — the thing an operator runs. One
  installation serves one organization.
- **Company** is a record type. Confusingly, the underlying name for it is
  "organization", which shows through on a few screens like search results. When
  you see "Organizations" in a search grouping, it means companies.

## Seats

Two kinds.

**Full seat.** Can read and change things, subject to role and row scope.

**Read seat.** Can read, and can never change anything, whatever the role says.
The seat is checked *before* the role, so no combination of permissions gets
around it. The refusal is plain: "This seat is read-only, so the request was
refused. Ask an operator to raise the seat."

**Only full seats are counted against your licence.** Read seats are unlimited
and never metered.

Agents get seats too. An agent seat is always a full seat and always counted,
because an agent acts on your data exactly as a person does — excluding them
would let an installation work without limit through agents.

**Your installation does not come with one.** Every seat it is billed for
belongs to a person. An agent identity has no role and no password: it is an
identity, not an authority, and what it may do comes entirely from the passports
people mint and the connections they approve for it.

### The licence

**Settings → License** shows **Seats in use** against **Seats granted**. If
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
| **Ops** | every normal record, and all configuration | everything — the same grid as Admin |
| **Management** | every record in the organization | every record; no administrative power |
| **Team Lead** | records | records they own; configuration read-only; no access to exchange rates, model prices, imports or retention |
| **User** | records | records they own; configuration read-only; full control of their own saved views |
| **Read-only** | every record and most configuration | nothing, except their own saved views |

A member with **no** role sees nothing at all. Every gate fails closed. If
someone reports an empty app, check their role first.

Holding more than one role gives you the widest of them.

The six are seeded with the product, but they are not frozen — an administrator
can change what they grant.

## Row scope: which records, not which kinds

Three levels: **own**, **team**, **all**.

- **own** — records you own, plus records nobody owns
- **team** — those, plus records owned by your teammates
- **all** — everything in the organization

Here is the part that differs from most CRMs, and it is deliberate:

> **Reading a contact, company, lead, deal or project ignores row scope
> entirely.**

Every seat holding the read permission reads every contact, company, lead, deal
and project in the organization. The app states it: "Reads every contact,
company, lead and deal in the organization."

Row scope governs **writes**. Not customer reads. Projects used to be the
exception and are not any more: a consultant delivering a project they neither
owned nor were granted got a 404, which is not a privacy boundary, only a
broken one.

Two things the word "everything" above does NOT cover, for any role including
Admin, because neither is a tier of row scope:

- **Correspondence you were not part of.** Mail and meetings carry their own
  audience, and seniority does not override it.
- **A captured contact still private to the person whose mailbox made it.** A
  connector creates a contact from a message nothing has judged yet, and it
  belongs to that seat alone until a classifier judges the sender or the owner
  publishes it themselves. An admin gets a 404, which is the point: connecting
  a mailbox with a year of history must not put every correspondent, a lawyer
  and a doctor among them, in front of the organization.

The reasoning is that a shared pipeline is the point. Two narrowings survive: a
record created by a connector can stay private to its owner until promoted, and a
per-record share can widen access further.

Your personal things — lists, saved views, automations, your writing voice — keep
the classic owner rule.

**No shipped role is team-scoped.** Team Lead and User both have *own* scope.
Being on a team with someone does not by itself let you edit their records; that
takes an explicit share or an unbounded seat.

An **ownerless** record is readable by everyone and writable by nobody until
somebody claims it.

Every record tells the app whether you can write it, so the edit buttons you see
match the answer the server would give. That is a convenience, not the
enforcement — the server checks again regardless.

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

A member who follows a link to an administrator page simply lands on their
Account page. The section is absent for them rather than announcing that it
exists and is not theirs.

## Teams

A team is a named group of people. Only an administrator creates one, renames it,
archives it, or changes who is in it. Anyone can see the list of teams.

**A team carries no permissions of its own.** It is not a role. It does two
things:

1. **It is a share target.** You can share a record with a whole team in one act.
   This is its main job today.
2. It resolves team-level row scope — which no shipped role uses.

Archiving a team stops its memberships resolving anything.

## Sharing one record

You can share a single record with a person or a team.

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
- An agent cannot share. It stages the share for approval instead.

There are no sharing hierarchies, no rules-based sharing, and no delegating the
right to share. Flat, explicit grants only. Every share and un-share is audited.

## Inviting and removing people

Only an administrator can, and only a person — an agent may never create a human
account.

**Inviting.** You choose a role, and the person is created with no password. If
your installation sends email, they get a link. If not, the administrator mints a
one-time link and hands it over directly: "Send this link to the member over a
channel you trust. It works once and is shown only now." The link lasts 7 days.

Before you send it, you can **preview what this person will see**, computed from
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

Everything above decides what people *can* do. The audit trail records what they
*did*: every action, attributed to a human, an agent or a connector, with the
authorization rule that allowed it.

Only an administrator reads it, because it names every actor and every record
they touched. See
[What is kept, what is destroyed](retention-exports-and-deletion.md#the-audit-trail).
