# Writing and sending mail

[Capture](capture.md) is how mail comes **in**. This page is how it goes **out**:
the composer, what is checked before a send lands, and scheduling one for later.

Who may then read what you sent is a separate question —
[Who can see an email](who-can-see-an-email.md).

## The composer

Reached from a contact, a company, a deal, a lead, or the reply action on any
message.

To and Cc stand on the form; **Bcc is a button until you ask for it**, and says
what it means: "The named recipients cannot see these addresses, and cannot see
that anyone else was copied."

A channel reply — Telegram — drops the subject and Cc, because the channel has
neither.

### Attaching files

Two sources, both behind the paperclip.

**On this record** offers the files already filed there — a shelf, not a file
browser, so it shows the most recent 25.

**Send a new file** uploads one, and files it on the record *first*: "It is
filed on this record first, so the timeline keeps what the message carried."

**Ten files is the limit for one message.** At the cap: "{most} files is the most
one message can carry. Send the rest as a second message."

A reply does not offer back the files that came in with the message you are
answering.

### Drafting with AI

**Draft with AI** writes from the record's own context, in your voice. It tells
you what it read — "Based on: {inputs}" — and **Why this draft?** opens the
reasoning.

Every draft is marked as one: "This draft was produced by AI. Read it and edit it
before you send."

Your voice profile carries a version, shown as "Built from your corpus · v{n}".
A profile still being built is marked **Provisional voice**, and the product is
careful to say what that does *not* mean: "It already shapes this draft exactly
as a finished one would — nothing is held back."

If the profile could not be loaded, it refuses to pretend:

> **This draft is not in your voice.** Your voice profile couldn't be loaded, so
> this draft is not written in your voice. Draft again, or edit before sending.

**Discard draft**: "Tells your Voice DNA this draft missed. The generated text is
never kept."

### The four rewrites

Over text the model wrote and you have not edited yet: **Shorter**, **Warmer**,
**More formal**, **Add a deadline**.

They stop being offered the moment you type, because they rewrite the model's
text rather than yours.

### Why are you writing?

> **The record decides what is allowed; this says what you are doing so the
> answer can be checked against it.**

Eight answers: They asked me to get in touch · About a deal we are working on ·
A quote or proposal they asked for · Support for something they bought · About an
invoice or a payment · About their contract · About their account · Marketing.

**You are not asked at all when you are replying** to their own message: "This
continues their own message, so it needs no reason from you."

The reason drives the consent check, which runs *before* you send and shows one
of three answers: ready to send, unproven, or refused. An unanswered check is
never read as permission.

Note what is deliberately missing from that list. Several purposes the system
recognises are not offered here, because a sender who could claim one could dress
marketing up as a security notice.

## What a send refuses, and what it merely warns about

**No consent** is a refusal:

> **Send blocked — no consent** — "A recipient has not granted consent for this
> purpose, so the send was suppressed (default-deny)."


The reason is specific, and several of them say plainly that nobody here can
overrule them — "They asked not to receive marketing. Nobody here can lift that,
including an administrator." Others name the actual fix: a bouncing address wants
correcting, not overriding; two records sharing one address want merging.

**A bouncing address is a warning, not a block.** "Mail to {addresses} is
bouncing… Send anyway, or use another address."

**A colleague's mailbox** is a note, not a block: your reply goes out from your
own mailbox, under your name.

**A mailbox that cannot send** is refused with the remedy: "Your mailbox is
connected for capture but was never granted permission to send. Reconnect it and
approve sending — a mailbox connected before sending existed cannot be upgraded
in place."

**A message carrying an unsubscribe link reaches one addressee at a time**,
because that link is the recipient's own consent record. Send it once per
recipient, with no Cc.

**Channel limits are explained rather than hit.** Telegram cannot carry files at
all, carries a limited number, caps each file's size, and truncates a caption —
and each of those is its own sentence naming the number.

## Your signature

Set at **Settings → Account**. Plain text, appended below every message you send
and above the unsubscribe footer. Leave it empty to send unsigned.

> **The AI never writes a sign-off — this is the one that goes out.**

An agent signs nothing at all.

The order in what actually leaves: your message, your signature, any required
disclosure, then the unsubscribe footer.

The copy kept on your timeline has the footer's **token redacted** — the shape is
there, the live link is not, because that link is the recipient's own credential.

## Scheduling a send

Behind the caret on Send: **Schedule send**.

Three presets — **Tomorrow morning**, **Tomorrow afternoon**, **Monday morning**
— or pick a date and one of four times.

The confirmation is explicit about what has and has not happened:

> This does not go out now. It waits for the moment you picked, and the consent
> and mailbox checks run again then. Until it goes you can move it or take it
> back from Scheduled messages.

That re-check is the point. A scheduled send is not a decision made today and
executed blindly later.

### The scheduled list

**"Messages you have written that have not gone out yet. Only you can see them."**
It is your own list, never the company's.

It is not on the navigation rail — reach it from the command palette, or from the
toast after you schedule something.

Three groups: **Stopped, waiting on you** · **Waiting to send** · **No longer
waiting**.

You can **Change moment** — the time only. Content is what the checks were made
against, so changing that means withdrawing and writing again.

**Withdraw** is the other verb, and it is called that rather than "delete":
"“{subject}” will not be sent, and nothing will reach the timeline. Writing it
again means composing it from scratch."

### Stopped

A message stops when the moment came and a check refused it. The reason says
which, and what to do:

- **A recipient withdrew consent** after you scheduled it.
- **Your seat or mailbox changed**, so it cannot be sent as you.
- **Its moment passed while nothing was running** — "it is now too late to be the
  message you wrote."
- **The job ran out of attempts.** Move it to a new moment to try again.
- **A check refused it.** Nothing was sent.

A stopped message also raises an approval card, and it is **the one card that
never expires**. The message is being held and nothing else will reap it, so the
card waits as long as it takes. See [Approvals](approvals.md).

## What the recipient controls

Every marketing message carries a footer with two destinations: unsubscribe, and
manage preferences. Both are the recipient's own private links.

**The preference centre** — "Each purpose is separate — this isn't all-or-nothing.
Transactional messages can't be switched off here, because you need them;
everything else is yours to control."

It shows three states per purpose, not two: on because they asked, on because
they have not objected, or off because they asked you to stop. **Only an explicit
objection reads as off.**

**Security & service messages are locked on**, and the page says why: "They're
needed for something you asked for — a password reset, or a confirmation you
requested."

**Stop all marketing** switches off every marketing row at once, and is careful
about what it does not touch: "Replies to your own enquiries, and anything you
asked us for, keep coming — nobody subscribed you to those, so there is nothing
there to switch off."

There is an **Undo** — and it does not silently re-subscribe anybody:
"Re-subscribing is an explicit opt-in — we won't silently turn it back on. Save
below to record your consent, or discard."

An unsubscribe link **never acts on arrival**. It shows a page and asks, because
a scanner following a link must not unsubscribe somebody.

Whatever they choose, the exact sentence they were shown is stored with the
decision: "We record the exact wording you saw and a timestamp as proof — then
it applies to every future send."

## Asking somebody to confirm their own details

From a contact: **Ask them to confirm their details**.

> Mails this contact a private link to see what you hold about them, correct it,
> and say whether they want to hear from you. It goes to their own recorded
> address; you cannot send it anywhere else.

The link lasts 14 days and works once. What they see is the whole record you
hold, where each field came from, a plain question about staying in touch, and a
way to ask for removal.

This is also the only way to start a purpose that needs confirming: "Only this
contact can confirm this purpose through a link sent to their recorded address."
A confirmation an employee can complete on the contact's behalf is not evidence
that the contact agreed.

## One honest gap

Choosing **Marketing** in the composer does not currently attach the purpose the
unsubscribe footer needs, so the link in that message lands the recipient on
"That link names nothing we send" rather than on the right page.

Until that is fixed, send marketing through a path that names its purpose, and
treat a marketing send straight from the composer as unfinished.
