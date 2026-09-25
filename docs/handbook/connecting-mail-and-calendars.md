# Connecting your mailbox and calendar

Capture reads mail and meetings only from a mailbox or calendar you connect.
This page is how to connect Gmail, Outlook, an IMAP mailbox or a calendar,
what each connection shows you, how to reconnect or disconnect one, and how
to import your older mail. What happens to a message once it arrives is in
[Capture](capture.md).

### How do I connect my Gmail or Outlook mailbox?
To connect your Gmail or Outlook mailbox to Margince, open **Settings → Connections**, press **Add connector**, and choose **Connect** beside **Gmail** or **Outlook**.
1. Open the account menu at the top right and choose **Settings**, then **Connections**.
2. In **Connected mailboxes and calendars**, press **Add connector**.
3. Press **Connect** beside **Gmail** or **Outlook** (a Microsoft work account).
4. Approve every permission on Google's or Microsoft's screen.
You then see "Connected. Your mailbox is capturing." An agent cannot connect a mailbox for you.
Also called: hook up Google Mail, link my inbox, email sync, Outlook sync, Office 365.

### How do I connect an IMAP mailbox?
To connect any other mail host to Margince, open **Settings → Connections**, press **Add connector**, choose **Connect** beside **IMAP mailbox**, and fill in the **Connect IMAP mailbox** form.
1. Open **Settings**, then **Connections**, and press **Add connector**.
2. Press **Connect** beside **IMAP mailbox**.
3. Fill **IMAP server**, **Port**, **Email address**, **App password**, **Mailbox** and **Messages per sync**, then press **Connect**.
Use an app-specific password. Missing fields show "Required: {fields}". An IMAP mailbox captures only: Margince cannot send from it, and it has no history import.
Also called: connect Fastmail, connect my own mail server, app password.

### How do I connect my calendar?
To connect your calendar to Margince, open **Settings → Connections**, press **Add connector**, and choose **Connect** beside **Google Calendar** or **Outlook Calendar**; a calendar connects separately from mail.
1. Open **Settings**, then **Connections**.
2. Press **Add connector**.
3. Press **Connect** beside **Google Calendar** ("Your Google Calendar, connected separately from Gmail.") or **Outlook Calendar** ("Your Outlook calendar, connected separately from Outlook mail.").
4. Approve access on the provider's screen.
Connecting Gmail does not connect Google Calendar, and the other way round.
Also called: calendar sync, sync meetings, link my agenda.

## What you can connect

You connect these yourself, at **Settings → Connections**, under **Connected
mailboxes and calendars**. **Add connector** lists only the providers you have
not connected yet.

| Connection | What it brings | Can send? |
|---|---|---|
| **Gmail** | "Mail sent and received in Gmail. Margince can also send from it." | Yes |
| **Google Calendar** | "Your Google Calendar, connected separately from Gmail." | No |
| **Outlook** | "Mail sent and received on a Microsoft work account. Margince can also send from it." | Yes |
| **Outlook Calendar** | "Your Outlook calendar, connected separately from Outlook mail." | No |
| **IMAP mailbox** | "Any other mail host, using an app password. Capture only." | No |
| **Telegram** | One bot for the whole company. An administrator connects it, not you. | Yes |

A mailbox connected before sending existed **cannot be upgraded in place**. The
provider only grants sending on a fresh connection, so you have to reconnect.
The mailbox shows **Capture only, no sending** and says so, rather than letting a
send fail mysteriously.

### What each connection shows you
Each connected mailbox or calendar shows one status:

- **Capturing** — working
- **Pending confirmation** — not yet confirmed live
- **Needs reconnect** — "The provider rejected the stored credentials. Reconnect
  to resume."
- **Sync error** — with a plain reason: being throttled, unreachable, or a
  history window that expired. Most of these say "nothing is lost" or retry
  automatically.
- **Disconnected**

You also see "Last synced", "Next check around", and whether it is polled on a
schedule or has a push subscription.

### Why did my Gmail or Outlook connection stop working?
When emails stop coming into Margince, open **Settings → Connections** and read the status on the mailbox under **Connected mailboxes and calendars**.
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect to resume." Press **Reconnect** and approve access again.
- **Sync error**: the line under it says why. Throttling and an unreachable provider retry by themselves. If it stays in **Sync error**, press **Disconnect**, then connect again with **Add connector**.
- **Disconnected**: connect again with **Add connector**.
Also called: mailbox not syncing, emails stopped coming in, Gmail disconnected, sync broken.

### How do I reconnect a mailbox?
To reconnect a mailbox in Margince — for example when Gmail stopped syncing — open **Settings → Connections** and press **Reconnect** on the mailbox, then approve access again.
1. Open **Settings**, then **Connections**.
2. Find the mailbox marked **Needs reconnect** or **Capture only, no sending**.
3. Press **Reconnect** and approve every permission. An IMAP mailbox reopens the **Connect IMAP mailbox** form instead.
**Reconnect** appears only on those two. Reconnecting is also how a capture-only mailbox gains permission to send.
Also called: re-authenticate, fix mailbox sync, refresh email connection.

### How do I disconnect a mailbox?
To disconnect a mailbox from Margince, open **Settings → Connections**, press **Disconnect** on the mailbox, and confirm with **Disconnect** in "Disconnect this mailbox?".
1. Open **Settings**, then **Connections**.
2. Press **Disconnect** on the mailbox or calendar.
3. Confirm with **Disconnect**.
Capture stops at once and the stored credential is deleted. Everything already captured stays in the CRM.
Also called: remove my mailbox, stop email sync, unlink Gmail.

### Disconnecting

> This deletes the stored credential for this mailbox. Capture stops
> immediately; captured records stay in the CRM, and reconnecting asks for
> permission again.

Note the honest footnote: Google or Microsoft may still list Margince under your
account's third-party access or connected apps. Remove it there too if you want
access fully revoked.

### An agent can never connect a mailbox for you

Connecting a mailbox is a human-only action. An agent granting itself read access
to a colleague's personal mail is exactly what this product does not allow.

A connection also cannot exceed the colleague who made it. If your permissions are
reduced later, the connection's reach is reduced with them on its next poll.

### How do I import my old emails?
To import your old emails into Margince, open **Settings → Connections** and use **Import mailbox history** under the connected mailbox: choose an **Import window** and press **Start import**.
1. Open **Settings**, then **Connections**.
2. Under the mailbox, in **Import mailbox history**, pick an **Import window** from **3 months** to **10 years**. 6 months is the default.
3. Read the message count and **Estimated AI cost**, then press **Start import**, or **Skip mailbox history import**.
**Stop import** keeps what was captured so far. IMAP mailboxes have no history import, and the window can only be widened later.
Also called: backfill, import past mail, sync old emails, email history.

## Importing your mail history

Importing mailbox history is a one-time backward scan offered when you connect a
mailbox. You choose a window: **3 months, 6 months, 1 year, 2 years, 3 years, 5
years, 7 years or 10 years** — or skip it. Six months is the default.

Before it runs, it counts. The count reads message ids only, not bodies, and
gives you the number of messages in the window and an estimated AI cost. It
counts exactly up to 20,000 messages; a larger mailbox is reported as a floor
("At least {count} messages in that period") rather than a made-up estimate.

You can stop it: "Stopped. Everything captured so far is kept."

The window can only be **widened** later, never narrowed: "A wider window
already ran for this mailbox. The import window can only be widened." And when
the progress bar fills, the import is finished but the AI work is not —
classification runs hourly and enrichment daily afterwards.

**Everything it brings in is held to begin with.** A new mailbox is *Held until
classified*, so a backfill of five years of mail does not put five years of mail
in front of your colleagues: each thread stays with whoever was on it
until a classifier judges it ordinary business. That is also true of the records
the import mints — a contact created from a thread nothing has judged yet is
yours alone until the verdict clears it.

Which means a backfill is worth watching in two places once it finishes:
**Senders**, for what was concluded about each address it saw, and **Held
threads**, for the threads still held. Both are under Settings → Connections.
