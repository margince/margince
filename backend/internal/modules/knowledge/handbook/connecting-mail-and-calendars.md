<!-- prose:plain -->
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

### How do I connect an IMAP mailbox?
To connect any other mail host to Margince, open **Settings → Connections**, choose **Add account** and pick **Another mail host**. Then fill in the **Connect IMAP mailbox** form.
1. Open **Settings**, then **Connections**.
2. Choose **Add account**, pick **Another mail host**, then **Continue**.
3. Fill **IMAP server**, **Port**, **Email address**, **App password**, **Mailbox** and **Messages per sync**, then press **Connect**.
Use a password made for this one app. Missing fields show "Required: {fields}". An IMAP mailbox only captures: no calendar, Margince cannot send from it, and it has no past email import.

**Messages per sync** is the most one check reads. The first messages arrive a few minutes after you connect, not at once.
Also called: connect Fastmail, connect my own mail server, app password.

### How do I get an app password for IMAP?
To get an app password, turn on two-step sign-in for your mail account, then create an app password in its security settings.
- **Gmail**: turn on 2-Step Verification, then make one under Google Account, Security, App passwords. The server is `imap.gmail.com`, **Port** 993.
- **Outlook**: turn on two-step sign-in, then create one under Security, Advanced security options, App passwords. The server is `outlook.office365.com`, **Port** 993.

Both providers refuse your normal password over IMAP. If your company turns off IMAP or app passwords, connect **Outlook** instead.
Margince seals the password and never shows it again. **Disconnect** deletes it, and you can also revoke it at the provider.
Also called: app-specific password, IMAP password, Gmail app password.

### Why does my IMAP mailbox not connect?
When the **Connect IMAP mailbox** form refuses, the message under it says which of two things went wrong.
- "The mailbox rejected these credentials. Check host, email and app password.": a value is wrong, or you used your normal password.
- "The mail server could not be reached. Check the host and port.": the server name or port is wrong, or the server is down.
An IMAP server inside your own network is always unreachable, because Margince never connects to a private address.
Also called: IMAP login failed, credentials rejected, mail server unreachable.

### How do I connect my calendar?
To connect your calendar to Margince, tick **Calendar** when you connect your Google or Microsoft 365 account in **Settings → Connections**. Mail and calendar connect in one go.
1. Open **Settings**, then **Connections**.
2. For a new account, choose **Connect Google** or **Connect Microsoft 365** and keep **Calendar** ticked.
3. For an account already connected with mail only, choose **Connect calendar** in its **Calendar** row. Or choose **Change what it brings in** in the account's menu.
4. Approve access on the provider's screen. With Mail and Calendar both ticked, the provider asks twice: once for mail, then once for your calendar.
If the calendar is refused, mail still connects and the note says so, with **Connect calendar** to try again.
A calendar brings meetings from 90 days back to one year from now, and the window moves forward on its own. There is no history import for a calendar.
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

Under the account, each part (**New mail**, **Past email**, **Calendar**) has
its own word:

- **Up to date**
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect
  to resume."
- **Needs attention**: with a plain reason, such as being slowed down,
  unreachable, or a history window that expired. Most of these say "nothing is
  lost" or try again on their own.
- **Not connected**

You also see when it last synced.

### Why did my Gmail or Outlook connection stop working?
When emails stop coming into Margince, open **Settings → Connections** and read the badge on the account and the word on its **New mail** row.
- **Needs reconnect**: "The provider rejected the stored credentials. Reconnect to resume." Press **Reconnect** and approve access again.
- **Needs attention**: the line under it says why. When the provider is slowed down or unreachable, Margince tries again on its own. If it stays, choose **Disconnect** in the account's menu, then connect again.
- **Not connected**: connect again with **Connect** on the row, or **Add account**.
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

You can stop it with **Stop**: "Everything imported so far is kept."

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
threads**, for the threads still held. Both are under Settings → Capture
activity.
