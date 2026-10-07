<!-- prose:plain -->
# Connecting your mailbox and calendar

Capture reads mail and meetings only from a mailbox or calendar you connect.
What happens to a message once it arrives is in [Capture](capture.md).

### How do I connect my Gmail or Outlook mailbox?
To connect your Gmail or Outlook mailbox to Margince, open **Settings → Connections**, press **Add connector**, and choose **Connect** beside **Gmail** or **Outlook**.
1. Open the account menu at the top right and choose **Settings**, then **Connections**.
2. In **Connected mailboxes and calendars**, press **Add connector**.
3. Press **Connect** beside **Gmail** or **Outlook** (a Microsoft work account).
4. Approve every permission on the Google or Microsoft screen.
You then see "Connected. Your mailbox is capturing." An agent cannot connect a mailbox for you.
Also called: hook up Google Mail, link my inbox, email sync, Outlook sync, Office 365.

### How do I connect an IMAP mailbox?
To connect any other mail host to Margince, open **Settings → Connections** and press **Add connector**. Choose **Connect** beside **IMAP mailbox**, and fill in the **Connect IMAP mailbox** form.
1. Open **Settings**, then **Connections**, and press **Add connector**.
2. Press **Connect** beside **IMAP mailbox**.
3. Fill **IMAP server**, **Port**, **Email address**, **App password**, **Mailbox** and **Messages per sync**, then press **Connect**.
Use a password made for this one app. Missing fields show "Required: {fields}". An IMAP mailbox only captures: Margince cannot send from it, and it has no history import.
Also called: connect Fastmail, connect my own mail server, app password.

### How do I connect my calendar?
To connect your calendar to Margince, open **Settings → Connections** and press **Add connector**. Choose **Connect** beside **Google Calendar** or **Outlook Calendar**; a calendar connects apart from mail.
1. Open **Settings**, then **Connections**.
2. Press **Add connector**.
3. Press **Connect** beside **Google Calendar** or **Outlook Calendar**. Their rows read "Your Google Calendar, connected separately from Gmail." and "Your Outlook calendar, connected separately from Outlook mail."
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

A mailbox connected before sending existed cannot add sending in place. The
provider grants sending only on a new connection, so you have to reconnect.
Until then the mailbox shows **Capture only, no sending**.

### What each connection shows you
Each connected mailbox or calendar shows one status:

- **Capturing**: working
- **Pending confirmation**: not yet confirmed live
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect
  to resume."
- **Sync error**: with a plain reason, such as being slowed down, unreachable,
  or a history window that expired. Most of these say "nothing is lost" or try
  again on their own.
- **Disconnected**

You also see "Last synced" and "Next check around". It also says whether the
mailbox is checked on a schedule or the provider sends a notice when new mail
arrives.

### Why did my Gmail or Outlook connection stop working?
When emails stop coming into Margince, open **Settings → Connections** and read the status on the mailbox under **Connected mailboxes and calendars**.
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect to resume." Press **Reconnect** and approve access again.
- **Sync error**: the line under it says why. When the provider is slowed down or unreachable, Margince tries again on its own. If it stays in **Sync error**, press **Disconnect**, then connect again with **Add connector**.
- **Disconnected**: connect again with **Add connector**.
Also called: mailbox not syncing, emails stopped coming in, Gmail disconnected, sync broken.

### How do I reconnect a mailbox?
To reconnect a mailbox in Margince (for example when Gmail stopped syncing), open **Settings → Connections**. Press **Reconnect** on the mailbox, then approve access again.
1. Open **Settings**, then **Connections**.
2. Find the mailbox marked **Needs reconnect** or **Capture only, no sending**.
3. Press **Reconnect** and approve every permission. An IMAP mailbox opens the **Connect IMAP mailbox** form again instead.
**Reconnect** appears only on those two. Reconnecting is also how a mailbox that only captures gets permission to send.
Also called: re-authenticate, fix mailbox sync, refresh email connection.

### How do I disconnect a mailbox?
To disconnect a mailbox from Margince, open **Settings → Connections** and press **Disconnect** on the mailbox. Confirm with **Disconnect** in "Disconnect this mailbox?".
1. Open **Settings**, then **Connections**.
2. Press **Disconnect** on the mailbox or calendar.
3. Confirm with **Disconnect**.
Capture stops at once and Margince drops its access to the mailbox. Everything already captured stays in the CRM, and reconnecting asks for permission again.
Google or Microsoft may still list Margince under the connected apps of your account. Remove it there too to end all access.
Also called: remove my mailbox, stop email sync, unlink Gmail.

### An agent can never connect a mailbox for you

Only you can connect your mailbox. An agent cannot.

A connection also cannot reach past the colleague who made it. If your
permissions are cut later, the connection's reach is cut with them on its next
check.

### How do I import my old emails?
To import your old emails into Margince, open **Settings → Connections** and use **Import mailbox history** under the connected mailbox. Choose an **Import window** and press **Start import**.
1. Open **Settings**, then **Connections**.
2. Under the mailbox, in **Import mailbox history**, pick an **Import window** from **3 months** to **10 years**. 6 months is the default.
3. Read the message count and **Estimated AI cost**, then press **Start import**, or **Skip mailbox history import**.
**Stop import** keeps what was captured so far. IMAP mailboxes have no history import, and the window can only be widened later.
Also called: backfill, import past mail, sync old emails, email history.

## Importing your mail history

Importing mailbox history is a one-time look back over old mail, offered when
you connect a mailbox. You choose a window: **3 months, 6 months, 1 year, 2
years, 3 years, 5 years, 7 years or 10 years**, or skip it. Six months is the
default.

Before it runs, it counts the messages in the window, without reading what they
say, and shows what the AI work will likely cost. Outlook gives a true count.
Gmail is counted up to 20,000 messages; a larger Gmail mailbox is shown as "At
least {count} messages in that period".

You can stop it: "Stopped. Everything captured so far is kept."

The window can only be **widened** later, never made smaller: "A wider window
already ran for this mailbox. The import window can only be widened." When the
progress bar fills, the import is finished but the AI work is not. Sorting runs
every hour and enrichment every day after that.

**Imported mail starts out held.** A new mailbox is *Held until
classified*, so importing five years of mail does not show it to your
colleagues. Each thread stays with whoever was on it until a sorting model
judges it ordinary business. The same holds for contacts the import creates. A
contact from a thread that nothing has judged yet is yours alone until the
verdict clears it.

Once an import finishes, check two places:
**Senders**, for what was decided about each address it found, and **Held
threads**, for the threads still held. Both are under Settings → Connections.
