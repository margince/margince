<!-- prose:plain -->
<!-- describes: backend/internal/modules/capture/health.go frontend/src/app/capturehealthbanner.tsx frontend/src/features/capture/connectors.tsx frontend/src/features/capture/imap-connect-form.tsx frontend/src/features/capture/imap-send-fields.tsx frontend/src/features/capture/connectors.telegram.tsx frontend/src/features/capture/connectors.telegrambusiness.tsx frontend/src/features/capture/backfill.tsx backend/internal/modules/capture/registry_connections.go backend/internal/modules/capture/oauthflow backend/internal/modules/capture/backfill.go backend/internal/modules/capture/historybaseline.go frontend/src/features/capture/connection-service.tsx frontend/src/features/capture/connection-stop.tsx backend/internal/modules/capture/gdocs backend/internal/modules/capture/meetingnotes.go backend/internal/modules/capture/meetingnotesread.go backend/internal/modules/capture/meetingnotesmail.go backend/internal/modules/activities/meetingnotes.go backend/internal/modules/activities/meetingnotesmail.go backend/internal/compose/connectors_sharedconsent.go backend/internal/compose/connectors_picture.go -->
# Connecting your mailbox and calendar

Capture reads mail and meetings only from an account you connect. This page
shows how to connect Google, Microsoft 365 or another mail host. It also shows
what each connection tells you, how to reconnect or disconnect one, and how to
import your older mail. What happens to a message once it arrives is in
[Capture](capture.md).

### How do I connect my Gmail or Outlook mailbox?
To connect your Gmail or Outlook mailbox to Margince, open **Settings → Connections** and choose **Connect Google** or **Connect Microsoft 365** in **Mail and calendar**.
1. Open the account menu at the top right and choose **Settings**, then **Connections**.
2. Choose **Connect Google** or **Connect Microsoft 365** (a Microsoft work account). Once one account is connected, use **Add account** instead and pick the account.
3. Under **What to bring in**, keep **Mail** and **Calendar** ticked, or untick one. Pick how much past email to import, or **None**. Choose **Continue**.
4. Choose **Continue to Google** or **Continue to Microsoft 365** and allow every permission on the provider's screen.
You land back on Connections with a note such as "Google connected" and "Mail and Calendar are syncing." An agent cannot connect a mailbox for you.
If the account says "{provider} is not configured on this installation.", an administrator has to set it up first.
Also called: hook up Google Mail, link my inbox, email sync, Outlook sync, Office 365.

Google also shares your account's picture. If that Google account is the address you sign in with, colleagues see its picture next to your name. A picture you uploaded or removed stays as it is. A shared mailbox, such as sales@, sets no one's picture.

### How do I connect an IMAP mailbox?
To connect any other mail host to Margince, open **Settings → Connections**, choose **Add account** and pick **Another mail host**. Then fill in the **Connect IMAP mailbox** form.
1. Open **Settings**, then **Connections**.
2. Choose **Add account**, pick **Another mail host**, then **Continue**.
3. Fill **IMAP server**, **Port**, **Email address**, **App password**, **Mailbox** and **Messages per sync**.
4. To send mail from this mailbox too, tick **Send from this mailbox**. Then press **Connect**.
Use a password made for this one app. Missing fields show "Required: {fields}". An IMAP mailbox has no calendar and no past email import.

**Messages per sync** is the most one check reads. The first messages arrive a few minutes after you connect, not at once.
Also called: connect Fastmail, connect my own mail server, app password.

### How do I send from an IMAP mailbox?
To send from an IMAP mailbox, tick **Send from this mailbox** on the **Connect IMAP mailbox** form. Margince then sends through the mailbox's SMTP server.
- **SMTP server**: leave it empty to use the IMAP server.
- **Security**: **SSL/TLS** uses port 465, **STARTTLS** uses port 587.
- **SMTP port**: leave it empty to use the port that goes with **Security**.

Margince logs in to the SMTP server with the same app password before it saves anything. Each mail you send gets a copy in the mailbox's **Sent** folder.
A mailbox you connected without the tick stays read-only. To turn sending on later, press **Reconnect** and tick the box.
Mail an agent drafts still waits for your approval, as it does for Gmail and Outlook.
Also called: SMTP, send through my own mail server.

### How do I get an app password for IMAP?
To get an app password, turn on two-step sign-in for your mail account, then create an app password in its security settings.
- **Gmail**: turn on 2-Step Verification, then make one under Google Account, Security, App passwords. The server is `imap.gmail.com`, **Port** 993.
- **Outlook**: turn on two-step sign-in, then create one under Security, Advanced security options, App passwords. The server is `outlook.office365.com`, **Port** 993.

Both providers refuse your normal password over IMAP. If your company turns off IMAP or app passwords, connect **Outlook** instead.
Margince seals the password and never shows it again. **Disconnect** deletes it, and you can also revoke it at the provider.
Also called: app-specific password, IMAP password, Gmail app password.

### Why does my IMAP mailbox not connect?
When the **Connect IMAP mailbox** form refuses, the message under it says what went wrong.
- "The mailbox rejected these credentials. Check host, email and app password.": a value is wrong, or you used your normal password.
- "The mail server could not be reached. Check the host and port.": the server name or port is wrong, or the server is down.
- "The SMTP server rejected this login. Check the server, the security and the app password.": sending is ticked, and the SMTP server refused the login.
- "The SMTP server could not be reached. Check the server and port.": sending is ticked, and its name, port or security is wrong.
- "To send from this mailbox, its email address must be a plain address, such as name@example.com.": the login is not an email address.
A mail server inside your own network is always unreachable, because Margince never connects to a private address.
Also called: IMAP login failed, credentials rejected, mail server unreachable, SMTP login failed.

### How do I connect my calendar?
To connect your calendar to Margince, tick **Calendar** when you connect your Google or Microsoft 365 account in **Settings → Connections**. Mail and calendar connect in one go.
1. Open **Settings**, then **Connections**.
2. For a new account, choose **Connect Google** or **Connect Microsoft 365** and keep **Calendar** ticked.
3. For an account already connected with mail only, choose **Connect calendar** in its **Calendar** row. Or choose **Change what it brings in** in the account's menu.
4. Approve access on the provider's screen. Google asks once for **Mail** and **Calendar** added together, and Microsoft 365 asks once for each.
If the calendar is refused, mail still connects and the note says so, with **Connect calendar** to try again.
A calendar brings meetings from 90 days back to one year from now, and the window moves forward on its own. There is no history import for a calendar.
Also called: calendar sync, sync meetings, link my agenda.

### How do I let Margince read my Google Meet notes?
Margince can read the notes and transcripts Google Meet writes. You connect this in **Settings → Connections**.
1. Open **Settings**, then **Connections**.
2. In your Google account, find the **Meeting notes** row and choose **Connect meeting notes**.
3. Approve access on the Google screen. Google asks for this apart from mail and calendar.

Margince may then open a Google Doc by its address, and nothing else in your Drive. It cannot list or search your files. It reads only the notes and transcripts Google Meet adds to a meeting in your calendar.

Once Google adds the notes to a meeting, Margince puts their text on the meeting as a transcript. It marks a meeting that still waits for an outcome as held, and suggests the tasks it finds in the transcript. An outcome already recorded stays as it is.

Each document is read once. Margince also does this for notes it already read. Margince reads notes only for meetings of the last 14 days that someone at your company set up. Everyone who can see the meeting can see its transcript. A document taken off the meeting is not read.

Connect meeting notes with the same Google account as your calendar. Notes are read only for meetings in that account's calendar, and the row says so when the two accounts differ.

Google may show **Error 400: redirect_uri_mismatch** when you connect meeting notes. Then ask your admin to add the meeting-notes address to the Google app and turn on the Google Docs API. The admin finds the addresses under Settings → **Sign-in and security** → **Sign-in apps** → **Set up Google app**. Reconnecting your account does not help until then.

If you untick the permission on the Google screen, the row says **Cannot read meeting notes** and the account offers **Reconnect**. The same happens when Google later refuses it.

To stop it, choose **Stop reading meeting notes** in the row. Mail and calendar keep running, and notes already on a meeting stay. Disconnecting the whole account stops it too.

A meeting with notes waiting to be read offers **Connect meeting notes** in its **Files** panel.
It says when your Google accounts differ. The message leaves once the transcript lands or the reading window ends.

### What happens to the "Notes by Gemini" email?
Google emails the notes to the guests of the meeting. Margince keeps that email private to those guests. It files the email under no contact, so the contact with your own address does not hold every set of notes.

When the subject names one of your meetings, Margince puts the notes on that meeting as its transcript. The meeting must have started in the 24 hours before the email, and the title must be the same, in any case. The contacts and readers of the meeting get it, and Margince suggests the tasks it finds.

Margince adds nothing if two meetings fit, or if the meeting already has a transcript. It does the same for a forwarded email, or one sent to someone else. It does this only for email Gmail confirms came from Google. This needs no Meeting notes connection, only your connected mailbox and calendar.

Also called: Google Docs, Meet transcripts, Gemini notes, meeting minutes.

### Who a meeting from your calendar shows

A meeting from your calendar names every guest on the invitation under **Attendees**, whether they accepted, declined or did not answer yet. A guest who is a contact links to their record. A guest who is not a contact yet shows by the name or address on the invitation. Colleagues show under **Colleagues**. A booked room is not a guest.

If you say no to an invitation, the meeting stays on for colleagues who are still going. It is called off for everyone only when the colleague who set it up calls it off.

Each guest's answer shows next to their name: **Declined**, **Maybe** or **No answer**. A guest who accepted shows no label. The next calendar sync updates an answer that changed.

A colleague's own calendar decides their answer while it is connected. Once they disconnect their calendar, the next sync of a colleague's calendar that lists them updates their answer instead. When they connect again, their own calendar decides it again.

A colleague on the invitation can read the meeting. A colleague who declined is shown under **Attendees** and cannot read it.

A guest added to the invitation later joins the meeting at the next calendar sync. A guest taken off the invitation leaves it, and a colleague taken off can no longer read it. When the calendars of several colleagues show the same guest, the guest stays until the last of those calendars drops them. Whoever set the meeting up decides alone: a guest or colleague they take off leaves it at their next calendar sync. The calendar of a colleague they took off does not need to sync first.

If a colleague deletes the meeting from their own calendar, they leave it, and so does each colleague no other connected calendar shows. The colleague whose calendar first filed the meeting stays on it. A guest from outside the team whom only their calendar showed stays on the meeting, as a record of who was invited.

If someone set who can see the meeting by hand, a colleague added later is shown but cannot read it unless they are picked. A meeting you imported from another CRM gets its guests from your calendar the same way.

An invitation with more than 50 guests is a mailing list to Margince. It keeps your colleagues on it and names up to 200 outside guests by the name or address on the invitation. Those guests are not linked to a contact, no contact is made for them, and the meeting does not count as contact with anyone. Past 200, the meeting says how many more guests it does not list.

When whoever set up the meeting hides the guest list, your copy of the invitation names only that host and you. Margince files that meeting, and only you and the host can read it. It shows on a guest's record once the host's calendar is connected, because that copy names every guest. Margince does not take guests from an invitation email, because the sender types that list.

## What you can connect

You connect these yourself, at **Settings → Connections**, under **Mail and
calendar**. One account is one row, with what it brings in listed under it.
**Add account** offers only the accounts you have not connected yet.

| Account | What it brings | Can send? |
|---|---|---|
| **Google** | Mail from Gmail and meetings from Google Calendar, either or both. Meeting notes from Google Meet, if you connect them. | Yes |
| **Microsoft 365** | Mail and calendar from Outlook on a Microsoft work account, either or both. | Yes |
| **Another mail host** | "IMAP with an app password. Mail only, no calendar. Sending is optional." | If you tick **Send from this mailbox** |

The company's **Telegram** bot lives on **Settings → Integrations**. It is one
bot for the whole company, connected by an administrator.

A mailbox connected before sending existed cannot add sending in place. The
provider grants sending only on a new connection, so you have to reconnect.
Until then the mailbox shows **Capture only, no sending**.

### What each connection shows you
Each account carries one badge:

- **Connected**: everything it brings in is working.
- **Syncing**: a past email import is running.
- **Needs attention**: one of its parts needs you. The row under it says which.
- **Not connected**

Under the account, each part (**New mail**, **Past email**, **Calendar**,
**Meeting notes**) has its own word:

- **Up to date**
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect
  to resume."
- **Needs attention**: with a plain reason, such as being slowed down,
  unreachable, or a history window that expired. Most of these say "nothing is
  lost" or try again on their own.
- **Not connected**

You also see its last good sync. A failed check does not move that time.

### Does a record show all my mail?

Dates on a contact or deal describe the mail on record. They do not prove that nobody replied outside these records.
A good sync or a finished history import does not prove that every Inbox and Sent message is on record.
A connection problem shows on **Settings → Connections**.
A task you kept stays even when some mail is missing.

### Why did my Gmail or Outlook connection stop working?
When emails stop coming into Margince, open **Settings → Connections** and read the badge on the account and the word on its **New mail** row.
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect to resume." Press **Reconnect** and approve access again.
- **Needs attention**: the line under it says why. When the provider is slowed down or unreachable, Margince tries again on its own. If it stays, choose **Disconnect** in the account's menu, then connect again.
- **Not connected**: connect again with **Connect** on the row, or **Add account**.
- **Your role cannot record activities**: your role cannot capture mail. Ask them to change it. Capture starts again by itself.
Also called: mailbox not syncing, emails stopped coming in, Gmail disconnected, sync broken.

### How do I reconnect a mailbox?
To reconnect a mailbox in Margince (for example when Gmail stopped syncing), open **Settings → Connections**. Press **Reconnect** on the part that needs it, then approve access again.
1. Open **Settings**, then **Connections**.
2. Find the part marked **Needs reconnect** or **Capture only, no sending**.
3. Press **Reconnect** and approve every permission. An IMAP mailbox opens the **Connect IMAP mailbox** form again instead.
**Reconnect** is also in the account's menu. Reconnecting is also how a mailbox that only captures gets permission to send.
Also called: re-authenticate, fix mailbox sync, refresh email connection.

### How do I disconnect a mailbox?
To disconnect an account from Margince, open **Settings → Connections** and choose **Disconnect** in the account's menu. Confirm in "Disconnect {account}?".
1. Open **Settings**, then **Connections**.
2. Open the account's menu and choose **Disconnect**.
3. Confirm with **Disconnect**. Mail and calendar both stop.
To stop only one of them, choose **Change what it brings in** in the menu and untick it.
Everything already captured stays in the CRM, and connecting again asks for permission again.
Google or Microsoft may still list Margince under the connected apps of your account, so remove it there too.
Also called: remove my mailbox, stop email sync, unlink Gmail.
### An agent can never connect a mailbox for you

Only you can connect your mailbox. An agent cannot.

A connection also cannot reach past the colleague who made it. If your
permissions are cut later, the connection's reach is cut with them on its next
check.

## The company's Telegram bot

One Telegram bot receives and sends messages for the whole company. It sits on
**Settings → Integrations** in the **Telegram bot** card. Only an administrator or operations
user can connect or change it, and an agent never can.

A Telegram bot cannot start a chat. A contact writes to the bot first, and only
then can you reply to them there. What a reply looks like is in
[Writing and sending mail](sending-mail.md).

### How do I connect the company's Telegram bot?
To connect the Telegram bot, create a bot with BotFather, then press **Connect Telegram bot** on **Settings → Integrations**.
1. In Telegram, write to @BotFather, send `/newbot`, and copy the token it gives you.
2. In Margince, open **Settings**, then **Integrations**.
3. In the **Telegram bot** card, press **Connect Telegram bot**.
4. Paste the token into **Bot token** and press **Connect**.
You then see "Connected as @yourbot." and a **Done** button. The bot starts reading messages within a minute.

Messages already waiting for the bot arrive too. Telegram keeps unread bot messages for about a day, so nothing older comes in.
The card may say "Messaging channels are not configured on this installation." Then an administrator has to set it up first.
Also called: add a Telegram bot, link Telegram, BotFather token.

### Why did my Telegram bot not connect?
When connecting fails, the form shows **Bot not connected** with the reason. The form starts empty again, so paste the token once more.
- The token was refused: check the token BotFather sent you, and that nobody revoked it.
- A bot is already connected: the company has one bot. Use **Replace token** or **Disconnect** in its **…** (**Actions**) menu first.
- Telegram could not be reached: nothing changed, so try again later.
Also called: Telegram token rejected, bot not connected.

### How do I replace the Telegram bot token?
To replace the bot token, choose **Replace token** in the bot's **…** (**Actions**) menu and paste the new token.
1. Open **Settings**, then **Integrations**.
2. In the **Telegram bot** card, open the bot's **…** (**Actions**) menu and choose **Replace token**.
3. Paste the new token into **Bot token** in **Replace bot token**, and press **Replace token**.
The connection stays, with all captured chats and contacts. You can even point it at a different bot.
Replacing the token keeps the status. A bot in **Needs reconnect** or **Sync error** needs **Disconnect**, then connecting again.
Also called: change the bot token, new BotFather token, change the bot.

### How do I disconnect the Telegram bot?
To disconnect the Telegram bot, choose **Disconnect** in its **…** (**Actions**) menu and confirm in "Disconnect this bot?".
1. Open **Settings**, then **Integrations**.
2. In the **Telegram bot** card, open the bot's **…** (**Actions**) menu and choose **Disconnect**.
3. Confirm with **Disconnect**.
Margince deletes the stored token and stops reading the bot at once. Every message and contact it captured stays in the CRM.
Also called: remove the Telegram bot, stop Telegram capture, unlink Telegram.

### Why does my Telegram bot show Sync error or Needs reconnect?
**Needs reconnect** means Telegram refused the stored token, so get a new one from BotFather.
**Sync error** on the bot means another program reads the same bot's messages. Telegram allows only one reader per bot.

Find that program and stop it. It may be a second Margince, a test installation, or another tool using the same token.
Margince does not try again on its own. Once the other reader is gone, press **Disconnect**, then connect the bot again.
Also called: Telegram not syncing, bot stopped capturing.

### How do I link a Telegram Business account?
To capture a company's Telegram Business account, its owner links the company's bot inside the Telegram app, and an administrator then approves it in Margince. The **Telegram Business** card appears on **Settings → Integrations** once the Telegram bot is connected. Only an administrator or operations user sees it.
1. The account owner opens Telegram, then **Settings**, then **Telegram Business**, then **Chatbots**.
2. The owner enters the bot's @username, which the **Telegram Business** card shows, and chooses which chats the bot may see.
3. In Margince, open **Settings**, then **Integrations**. The account appears in the **Telegram Business** card as **Waiting for approval**.
4. Press **Approve as company account**, choose the team that reads the account's messages, and confirm.
Nothing from the account is captured until you approve it. After that, customer messages and the owner's replies from the Telegram app appear in the customer's **History**. Staff keep replying in Telegram, because Margince has no reply box for these chats.
A new account starts as **Held**: only the team you chose and everyone taking part in each chat can read its messages. Check with the owner which chats they selected before you approve. The team is required the first time, and an account that comes back to the seat it had keeps that team.
Also called: Telegram Business chatbot, approve a Telegram account, connect a business account.

### Who reads a Telegram Business account's messages?
Each approved account shows **Read by** its team and either **Held** or **Shared** on its row in the **Telegram Business** card.
- **Held** means only the team and everyone taking part in each chat can read the messages.
- **Shared** means everyone who can open the linked customer or deal can read them.
To change it, press **Change who reads** on the row, pick **Held** or **Shared**, and confirm in "Apply to all mail". The change covers messages the account already captured, not only new ones, and it works the same way as for a shared mailbox.
**Shared** needs the company's opt-in. Without it the dialog says so and nothing changes. An account that belongs to a single member shows **each chat is held** and has no team to change.
Also called: Telegram Business visibility, who sees business chats, reader team.

### What does Switched off in Telegram mean?
**Switched off in Telegram** means the owner removed the bot from their Telegram Business settings. Margince captures nothing new from that account, and what it already captured stays.
The account cannot be approved while it is switched off, and pressing **Approve as company account** then says so. To use it again, the owner links the bot once more, and the account waits for approval again.
Also called: Telegram Business account disconnected, owner removed the bot.

### How do I disconnect a Telegram Business account?
To stop capturing a Telegram Business account, press **Disconnect** on its row in the **Telegram Business** card and confirm.
Margince stops capturing from the account at once. Everything it already captured stays in the CRM. If the owner links the bot again, the account waits for approval again.
Also called: remove a business account, stop Telegram Business capture.

### How do I import my old emails?
To import your old emails into Margince, pick a window under **Import past email** when you connect. Or choose **Import older email** in the account's menu in **Settings → Connections**.
1. When you connect, pick a window from **3 months** to **10 years**, or **None**. 6 months is the default.
2. Once you are signed in, a small import starts on its own. The **Past email** row shows how far it is.
3. A larger one waits for you. The row says **Waiting for you**, shows how many emails it covers, and offers **Start import**.
4. Later, choose **Import older email** in the account's menu and pick **How far back**. Read the email count and the AI cost, and press **Start import**.
**Stop** keeps what was imported so far. IMAP mailboxes have no past email import, and the window can only be widened later.
Also called: backfill, import past mail, sync old emails, email history.

### How do I stop old mail from suggesting commitments?
To keep old mail from turning into suggested commitments, set **Suggest commitments from** on the mailbox. It is in the mailbox's settings under **Settings → Connections**, and in **Import old emails**.
1. Open **Settings**, then **Connections**, and open the mailbox.
2. Pick a date in **Suggest commitments from** and press **Save date**.
Mail sent before that day, in your own time zone, suggests no commitments for you. Mail sent on that day still does. The mail stays captured and you can still search it. A promise it holds is listed under **Commitment review** on the email.

The date applies only to this mailbox and only to your own commitments. A colleague who has the same email in their mailbox still gets their suggestions. Meeting transcripts stay as they are.

Changing the date or pressing **Clear date** completes and deletes nothing. Tasks you already have stay as they are.
Also called: ignore old promises, start date for suggestions.

## Importing your mail history

Importing past email is a one-time look back over old mail, offered when you
connect a mailbox. You choose a window: **3 months, 6 months, 1 year, 2 years, 3
years, 5 years, 7 years or 10 years**, or **None**. Six months is the default. A
small import starts by itself once you are signed in. A larger one, or one whose
count could not be taken, shows its estimate and waits for you to press **Start
import**.

Before it runs, it counts the messages in the window, without reading what they
say, and shows what the AI work will likely cost. It counts up to 20,000
messages. A larger mailbox is shown as "{count} emails or more. Counting stopped
there." and not as a made-up guess.

Internal mail is left out unless an Admin has authorized its source and history
range. You must also choose to include that scope in a new run. Read the scope before
starting. [Who can see an email](who-can-see-an-email.md) explains the separate
admin and mailbox-owner choices.

You can stop it with **Stop**: "Everything imported so far is kept."

The window can only be **widened** later, never made smaller: "A wider window
already ran for this mailbox. The import window can only be widened." When the
progress bar fills, the import is finished but the AI work is not. Sorting runs
every hour and enrichment every day after that.

**Imported mail starts out held.** A new mailbox is *Held until
checked*, so importing five years of mail does not show it to your
colleagues. Each thread stays with whoever was on it until a sorting model
judges it ordinary business. The same holds for contacts the import creates. A
contact from a thread that nothing has judged yet is yours alone until
Margince has sorted the thread.

Once an import finishes, check two places.
**Sender decisions** shows what was decided about each address it found. **Mail
not shared with the team** shows the threads still held. Both are under Settings →
Connections → Activity.

## Why is there a warning about my mailbox?

A warning at the top of every page names a connection that needs attention and says since when.
Choose **Reconnect** when its permission has expired, or **Open connection** to check it.
The same warning stays in your attention feed.

A mailbox warns when its sync stops or it delivers no mail for three days. For a mailbox
that has never delivered mail, the three days start when you connect it. A quiet mailbox
may be quiet by choice: dismiss its warning to keep it out of the banner and feed.
If mail arrives later and another three days pass without delivery, the warning returns.
Errors still appear after you dismiss this warning. Margince sends no email about it.

Admins and operations can see connections needing attention in **System health**, including
the member, mailbox and last good sync. They see no subjects or message contents there.
