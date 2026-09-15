# Your own settings

The five pages under **You** in Settings. Everything here changes your own seat
and nobody else's, with two named exceptions that carry one company-wide card
each — Connections and Capture activity.

For everything else, see [Settings](settings.md).


## Account

Your account as one card: who you are, how you sign in, how you sign off, which
language the product speaks to you in (English, German and Vietnamese), and how
it looks. **Appearance** is here as well as in the account menu — one setting
with two doors, so changing it in either moves the other.

**Your email signature** lives here. It is appended below every message you send,
above the unsubscribe footer. Leave it empty to send unsigned.

One rule worth repeating: **the AI never writes a sign-off.** This is the one
that goes out.

### When you are bookable

**Working hours** live on this page too: "Yours alone. Nobody sets these for you,
and you set them for nobody else."

One start time, one end time, the days you work, and the timezone those two times
are read on. Until you choose, customers are offered 09:00–17:00 Monday to
Friday on the installation's own clock — and the card says that is a default
rather than a decision you made.

Narrowing them says what it costs rather than just saving: "You are bookable for
less of the week. Fewer customers will find a time. That is the change, not a
fault."

Two things to know before you rely on this. Free/busy is worked out from
**meetings recorded in Margince**, not from your actual diary, even where a
calendar is connected. And the hours shape what a customer is *offered*; they are
not re-checked when a booking lands.

**The booking page is not usable yet.** The in-app one cannot complete a booking,
and there is no way for you to obtain a public booking link — one is seeded for
the installation's first administrator and nothing in the product hands it out.
Margince also sends no invitation on any path: "Margince does not send the invite
— tell your attendee the time yourself." Set your hours if you like; do not plan
a booking flow around them yet.

## Writing voice

Your **Voice DNA**: "Your personal writing voice. It shapes drafts made for you,
stays private to you, and only learns from sources you add."

Three properties in one sentence. It affects your drafts. Nobody else sees it.
It learns only from what you give it.

Samples arrive as files (`.txt`, `.md`, `.pdf`, `.docx`, `.vtt`, `.srt`,
`.json`, several at once). A PDF or Word document is read to its text in the
browser before anything is sent; a scanned PDF with no text layer counts as
empty, and a password-protected one is named so you can paste its text instead.
The card says what teaches the voice — sent emails first, then proposals and
posts, then call transcripts — and what to leave out: somebody else's writing,
and AI drafts.

A file at least half attributed to named speakers is treated as a conversation:
the card asks "Which speaker is you?" and keeps only your turns. Below that
share it is prose and is taken whole, so an email opening a line with a heading
and a colon ("Frage: …") is not asked about.

A first build needs 800 words. The button reads "Build my Voice DNA" until a
version exists, and "Rebuild Voice DNA" after.

## Agents

Where you mint and revoke **passports** — the credentials that let an AI agent
work as you.

Every member gets this page, ungated. A passport is minted by a colleague for
their own use, so making it administrator-only would mean only administrators
could mint one.

You also see the governed tool list and connected agents here. Disconnecting an
agent ends the whole connection, not one credential: "the agent loses access on
its next call and cannot renew. Reconnecting means approving access again."

See [Agents, passports and what they may
do](agents-and-passports.md#passports-how-an-agent-is-connected).

## Connections

Your own mailbox and calendar connections, and your LinkedIn import. The
distinction from **Integrations** is deliberate: Connections is what *you*
connected, Integrations is what the *installation* is wired to.

Full detail in [Capture](capture.md#what-you-can-connect).

## Capture activity

Two things on one page: the senders you keep out, and what the last 24 hours of
your mail turned into.

**Keep out of capture.** Addresses and domains whose messages never enter the
CRM. Rules you set bind only your own mailboxes; the company's rules bind
everyone (and only an administrator may add or remove one of those). Takes
effect from the next message; what is already captured stays.

**Outcomes.** Five counters for the window — captured, dropped as internal, no
contact created, sent for a verdict, derivation failed. Click one to narrow the
list under it.

**Messages**, behind a disclosure, is the per-message log: which step a single
message stopped at and why. Open it when a message you expected did not show up.
Most installations record no sender and no subject for these rows — the page says
so once above them, and that is the default, not a misconfiguration.

---

