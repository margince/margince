# Scheduling

How a meeting time is proposed, who decides which times are offerable, and which
clock that decision is made on.

This page exists because the feature was built without it. Working hours were
`9` and `17` in a Go constant, evaluated on a UTC clock, with no way for anyone
to change either — and the next increment was about to be an installation-wide
setting, which is the wrong shape for a reason worth writing down once.

## What the product offers

`Store.Availability` answers *which times is this host free*, for one host, one
window and one slot length. Two callers ask it: the authenticated surface (a rep
proposing times to a customer) and the public booking page, which resolves a
`HostUserID` from the booking link before it asks. **Both know whose calendar
they are asking about**, which is why every rule below can be a fact about a
PERSON rather than about the installation.

The answer is the window minus three things: times outside the host's working
hours, days the host does not work, and slots overlapping a meeting already on
their calendar.

## Working hours belong to the person

**Each person sets their own, and nobody sets them for anybody else** — the same
rule their display language follows.

An installation-wide pair of numbers set by an admin was the obvious next step
and is the wrong one. People on one team do not share working hours: they sit in
different countries, some work part time, some keep hours nobody else keeps. One
pair is wrong for most of them, and — this is the part that makes it worse than
no setting at all — the people it is wrong for cannot fix it. An unconfigurable
default is honestly wrong for everyone; an admin-set pair is authoritatively
wrong for the majority.

What a person sets is deliberately small:

| | |
|---|---|
| **One start time and one end time** | the same range on every day they work |
| **Which days they work** | any subset of the seven |
| **Their timezone** | the clock the two times are read on |

**One range rather than per-day hours**, because a single range plus working days
already covers both cases that prompted this — *8–18 Monday to Saturday* and
*9–13 Monday to Thursday* — and per-day hours can be added on top later without
redoing this shape. Several blocks in one day is calendar territory and should
not be built as a setting at all.

## Unset falls back, and the fallback is a decision

On the day this ships, everyone has set nothing. Of the three possible answers
to that, only one leaves the product working:

- Treating unset as *no constraint* lets a customer book somebody at 3am.
- *Requiring it before booking works* breaks the feature for every existing
  person until they act.
- **Falling back to 09:00–17:00, Monday to Friday, in the person's own zone**
  regresses nothing — and it makes the UTC bug disappear for everybody on day
  one, before anyone has touched a setting.

The fallback is a named default (`defaultWorkingHours`) rather than a leftover
constant, so it reads as the decision it is.

## Which clock

The hours are the person's own, so they are read on the person's own zone.

`app_user.timezone` is that zone. It was `NOT NULL DEFAULT 'UTC'` and nothing in
the product ever wrote it, so every value in it was the default rather than a
choice — which is why it is nullable now, exactly as `locale` beside it is:
**absent means nobody has chosen**, and a person who has not chosen is read on
the installation's reporting timezone rather than on UTC. A browser's own zone
pre-fills the field the first time a person opens the setting, so choosing is
usually confirming.

Before this, `freeSlots` read `cursor.Hour()` and `cursor.Weekday()` off a UTC
instant. "9am" therefore meant 4pm in Ho Chi Minh City, and a Monday morning in
Saigon was still Sunday to the scheduler.

That defect and its repair belong to two different owners, and the division
matters because either can land first:

- **"A calendar day must be derived in a named zone, not UTC"** is the general
  rule, and it is not this page's.
- **"Which zone, and whose hours"** is this page's, and the answer is *the
  person's* — not the workspace's. A repair that converted these reads to the
  workspace zone would be a correct timezone fix under the wrong owner, and
  would have to be undone here.

## How the module boundary is crossed

Working hours are a fact about a person, so `identity` owns the columns.
Availability is computed in `activities`, which may not import a sibling module.

So `activities` takes a resolver — *"given a host, what hours and what zone"* —
and `compose` injects the identity-backed one. A store with no resolver injected
answers with the fallback rather than refusing: the scheduling path is reachable
from the public booking page, and a wiring gap there must not turn into a
customer-facing error about somebody's settings.

## What the screen owes the person

A host who narrows their hours to 09:00–13:00 will receive roughly half the
bookings they do today, and will not necessarily connect the two.

**The screen says so at the moment they save.** That sentence is the difference
between a setting and a trap, and it is part of the feature rather than a polish
item on top of it.

## What is deliberately not here

- **Per-day hours**, and several blocks in a day. See above.
- **Holidays and time off.** A day a person is not working is a calendar fact,
  and this setting is not a calendar.
- **A real calendar connector.** Busy time is read from meetings this product
  holds; a host booked in an external calendar is free as far as this is
  concerned. `assumedMeetingDuration` exists because an activity carries only
  `occurred_at`, and both refine when a connector lands.
