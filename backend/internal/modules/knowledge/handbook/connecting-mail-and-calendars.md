# Connecting your mailbox and calendar

Capture reads mail and meetings only from an account you connect. This page is
how to connect Google, Microsoft 365 or another mail host, what each connection
shows you, how to reconnect or disconnect one, and how to import your older
mail. What happens to a message once it arrives is in [Capture](capture.md).

### How do I connect my Gmail or Outlook mailbox?
To connect your Gmail or Outlook mailbox to Margince, open **Settings → Connections** and choose **Connect Google** or **Connect Microsoft 365** in **Mail and calendar**.
1. Open the account menu at the top right and choose **Settings**, then **Connections**.
2. Choose **Connect Google** or **Connect Microsoft 365** (a Microsoft work account). Once one account is connected, use **Add account** instead and pick the account.
3. Under **What to bring in**, keep **Mail** and **Calendar** ticked, or untick one, and pick how much past email to import, or **None**. Choose **Continue**.
4. Choose **Continue to Google** or **Continue to Microsoft 365** and allow every permission on the provider's screen.
You land back on Connections with a note such as "Google connected" and "Mail and Calendar are syncing." An agent cannot connect a mailbox for you.
Also called: hook up Google Mail, link my inbox, email sync, Outlook sync, Office 365.

### How do I connect an IMAP mailbox?
To connect any other mail host to Margince, open **Settings → Connections**, choose **Connect it with an app password (IMAP)**, and fill in the **Connect IMAP mailbox** form.
1. Open **Settings**, then **Connections**. With an account already connected, choose **Add account** and pick **Another mail host**, then **Continue**.
2. Fill **IMAP server**, **Port**, **Email address**, **App password**, **Mailbox** and **Messages per sync**, then press **Connect**.
Use an app-specific password. Missing fields show "Required: {fields}". An IMAP mailbox captures only: no calendar, Margince cannot send from it, and it has no past email import.
Also called: connect Fastmail, connect my own mail server, app password.

### How do I connect my calendar?
To connect your calendar to Margince, tick **Calendar** when you connect your Google or Microsoft 365 account in **Settings → Connections**; mail and calendar connect in one go.
1. Open **Settings**, then **Connections**.
2. For a new account, choose **Connect Google** or **Connect Microsoft 365** and keep **Calendar** ticked.
3. For an account already connected with mail only, choose **Connect calendar** in its **Calendar** row, or **Change what it brings in** in the account's menu.
4. Approve access on the provider's screen. With Mail and Calendar both ticked, the provider asks twice: once for mail, then once for your calendar.
If the calendar is refused, mail still connects and the note says so, with **Connect calendar** to try again.
Also called: calendar sync, sync meetings, link my agenda.

## What you can connect

You connect these yourself, at **Settings → Connections**, under **Mail and
calendar**. One account is one row, with what it brings in listed under it.
**Add account** offers only the accounts you have not connected yet.

| Account | What it brings | Can send? |
|---|---|---|
| **Google** | Mail from Gmail and meetings from Google Calendar, either or both. | Yes |
| **Microsoft 365** | Mail and calendar from Outlook on a Microsoft work account, either or both. | Yes |
| **Another mail host** | "IMAP with an app password. Mail only, no calendar, no sending." | No |

The company's **Telegram** bot is not here. It is one bot for the whole company,
connected by an administrator on **Settings → Integrations**.

A mailbox connected before sending existed **cannot be upgraded in place**. The
provider only grants sending on a fresh connection, so you have to reconnect.
The mailbox shows **Capture only, no sending** and says so, rather than letting a
send fail mysteriously.

### What each connection shows you
Each account carries one badge:

- **Connected**: everything it brings in is working.
- **Syncing**: a past email import is running.
- **Needs attention**: one of its services needs you. The row under it says which.
- **Not connected**

Under the account, each service (**New mail**, **Past email**, **Calendar**)
has its own word: **Up to date**, **Needs reconnect** ("The provider rejected
the stored credentials. Reconnect to resume."), **Needs attention** with a plain
reason (being throttled, unreachable, or a history window that expired; most of
these say "nothing is lost" or retry automatically), or **Not connected**. You
also see when it last synced.

### Why did my Gmail or Outlook connection stop working?
When emails stop coming into Margince, open **Settings → Connections** and read the badge on the account and the word on its **New mail** row.
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect to resume." Press **Reconnect** and approve access again.
- **Needs attention**: the line under it says why. Throttling and an unreachable provider retry by themselves. If it stays, choose **Disconnect** in the account's menu, then connect again.
- **Not connected**: connect again with **Connect** on the row, or **Add account**.
Also called: mailbox not syncing, emails stopped coming in, Gmail disconnected, sync broken.

### How do I reconnect a mailbox?
To reconnect a mailbox in Margince, for example when Gmail stopped syncing, open **Settings → Connections** and press **Reconnect** on the service that needs it, then approve access again.
1. Open **Settings**, then **Connections**.
2. Find the service marked **Needs reconnect** or **Capture only, no sending**.
3. Press **Reconnect** and approve every permission. An IMAP mailbox reopens the **Connect IMAP mailbox** form instead.
**Reconnect** is also in the account's menu. Reconnecting is how a capture-only mailbox gains permission to send.
Also called: re-authenticate, fix mailbox sync, refresh email connection.

### How do I disconnect a mailbox?
To disconnect an account from Margince, open **Settings → Connections**, choose **Disconnect** in the account's menu, and confirm in "Disconnect {account}?".
1. Open **Settings**, then **Connections**.
2. Open the account's menu and choose **Disconnect**.
3. Confirm with **Disconnect**. Mail and calendar both stop.
To stop only one of them, choose **Change what it brings in** in the menu and untick it. Capture stops at once and the stored credential is deleted. Everything already captured stays in the CRM.
Also called: remove my mailbox, stop email sync, unlink Gmail.

### Disconnecting

> This stops {services} and deletes the stored credential. What was captured
> stays in the CRM, and connecting again asks for permission again.

Note the honest footnote: Google or Microsoft may still list Margince under your
account's third-party access or connected apps. Remove it there too if you want
access fully revoked.

### An agent can never connect a mailbox for you

Connecting a mailbox is a human-only action. An agent granting itself read access
to a colleague's personal mail is exactly what this product does not allow.

A connection also cannot exceed the colleague who made it. If your permissions are
reduced later, the connection's reach is reduced with them on its next poll.

### How do I import my old emails?
To import your old emails into Margince, pick a window under **Import past email** when you connect, or choose **Import older email** in the account's menu in **Settings → Connections**.
1. When you connect, pick a window from **3 months** to **10 years**, or **None**. 6 months is the default.
2. Once you are signed in, a small import starts on its own and the **Past email** row shows how far it has got. A larger one waits for you: the row says **Waiting for you**, shows how many emails it covers, and offers **Start import**.
3. Later, choose **Import older email** in the account's menu, pick **How far back**, read the email count and the AI cost, and press **Start import**.
**Stop** keeps what was imported so far. IMAP mailboxes have no past email import, and the window can only be widened later.
Also called: backfill, import past mail, sync old emails, email history.

## Importing your mail history

Importing past email is a one-time backward scan offered when you connect a
mailbox. You choose a window: **3 months, 6 months, 1 year, 2 years, 3 years, 5
years, 7 years or 10 years**, or **None**. Six months is the default. A small
import starts by itself once you are signed in; a larger one, or one whose count
could not be taken, shows its estimate and waits for you to press **Start
import**.

Before it runs, it counts. The count reads message ids only, not bodies, and
gives you the number of messages in the window and an estimated AI cost. It
counts exactly up to 20,000 messages; a larger mailbox is reported as a floor
("{count} emails or more. Counting stopped there.") rather than a made-up estimate.

You can stop it with **Stop**: "Everything imported so far is kept."

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
threads**, for the threads still held. Both are under Settings → Capture
activity.
