<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/account-settings.tsx frontend/src/features/settings/notification-settings.tsx frontend/src/features/sign-in/mfasetup.tsx frontend/src/features/booking/booking-setup.tsx frontend/src/features/voice-knowledge/voice-dna.tsx frontend/src/features/capture/connection-settings.tsx backend/internal/modules/identity/mfa.go backend/internal/modules/activities/scheduling_profile.go -->
# Your own settings

Your own settings in Margince are the pages under **You** in Settings:
**Account**, **Meetings**, **Writing voice**, **AI access**, **Notifications**,
**Connections** and **Capture activity**. Everything here changes your own seat and nobody else's.
Capture activity also carries one card that holds for the whole company: the
company's rules for what capture keeps out, beside your own. Connections only
shows the company's email sharing rule, on a row marked **Whole company**, and changes
nothing for anyone else.

For everything else, see [Settings](settings.md).

### How do I open my own settings?
To open your own settings in Margince, open the account menu at the top right and choose **Settings**, then pick a page under **You**.
The pages are **Account**, **Meetings**, **Writing voice**, **AI access**, **Notifications**, **Connections** and **Capture activity**. **Account** holds your name, password, signature, language and look. **Meetings** holds your booking link, calendar, working hours and what guests see.
You can also press ⌘K (Ctrl+K) and type the page name, or a word such as "password", "language", "theme" or "mailbox".
Also called: my profile, my account, preferences, user settings.

### How do I finish setting up Margince?
To see what is left of your setup in Margince, open Settings and look at **Your setup** on **Overview**.
It lists the steps: connect your mail, connect your calendar, add writing samples and turn on your booking link. Each step has a button, such as **Connect mail** or **Turn on**.
While steps are open, **Finish setup** at the top of every screen shows how many are done. Choose **Hide until next sign-in** to put it away for now.
Also called: setup list, getting started, onboarding steps.

### How do I save a change on a settings page?
To save a change on a settings page in Margince, change a field, then choose **Save** in the bar at the foot of the page.
**Discard** in that bar puts every field back as it was. If a field has a mistake, the bar says "Fix the marked fields to save."
Switches work at once and need no save. If you leave with changes not saved, Margince asks "Save your changes?" and offers **Save**, **Discard changes** or **Stay**.
Also called: save settings, changes not saved, undo changes.

### How do I change my notifications?
<!-- opens: settings/notifications -->
To change your notifications in Margince, open Settings → **Notifications**.
1. Under **Delivery by kind**, pick how each kind reaches you: **Off**, **In the app**, **Email** or **Daily digest**. Not every kind offers every choice.
2. Under **Email summaries**, set **Morning brief** and **Weekly review** to **By email** or **Not by email**.
3. Under **Browser notifications**, press **Allow**, then allow it in the browser's own question.
   The browser then shows a due reminder on one of your tasks while Margince is open in a tab.
   To turn it off, block notifications for this site in your browser settings. If the browser
   blocks them, the row says so and offers no **Allow**.

Every notice also stays in the notification center under the bell, whatever you pick. **Nudges from colleagues** cannot be turned off. The brief and the review always appear on **Home**, and what you still need to do waits on your Worklist there.

Morning and weekly emails use your saved account language. Without a saved
language, they use the company language. Password links use the account
language too; invitations for a new account use the company language.

Weekly emails show recorded results and name open due tasks separately.
An empty week with no work to follow up means no email is sent. A team email opens
the selected team and week. If part of the report could not be read, the email
says so rather than counting missing information as zero.

Also called: email notifications, turn off emails, alerts, daily digest, browser notifications.

## Account

The Account page in Settings has three panels. **Profile** holds who you are,
your display and greeting names, and your email signature. **Security** holds your
password and two-step sign-in, and **Sign out**. **Language and appearance**
holds which language the product uses with you (English, Deutsch or Tiếng
Việt), and how it looks.
**Appearance** is here as well as in the account menu, where it is called
**Theme**. Both change the same setting.

Where your browser can add Margince to this device as an app, a fourth panel
shows: **This device**. It offers **Install**, says how to add the app, or says
**Installed**.

### How do I change my password?
<!-- opens: settings/account -->
To change your password in Margince, open Settings → **Account** and choose **Change password** in the **Password** row.
1. Enter your **Current password**.
2. Enter a **New password** of at least 12 characters, and type it again in **Confirm new password**.
3. Choose **Save new password**.

Margince says **Password changed** and signs you out on all your other computers and phones.
Forgot it? Use **Forgot password?** on the sign-in page where it is shown, or ask an administrator for a new link to set a password.
Also called: reset my password, update login.

### How do I turn on two-step sign-in?
<!-- opens: settings/account -->
To turn on two-step sign-in in Margince, open Settings → **Account** and choose **Set up** in the **Two-step sign-in** row.
1. Open an app on your phone that makes sign-in codes, and scan the picture on the screen.
2. If you cannot scan it, choose **Copy key** and add the **Secret key** to the app instead.
3. Type the code the app shows into **Code from the app**, and choose **Turn on**.
4. Keep the backup codes under **Recovery codes** in a place only you can reach. **Copy codes** helps.
5. Choose **Done**.

Margince shows the backup codes once, and never again. The row then reads "On · 10 recovery codes left".
When two or fewer are left, the row warns you.
Turning it on signs you out on all your other computers and phones.
Also called: MFA, second sign-in step, sign-in code, code app.

A setup you started and did not finish shows **Not finished**. **Discard setup** removes it, with no code needed.

### What if Margince asks me to set up two-step sign-in before I can continue?
Your installation requires two-step sign-in, and you have not set it up yet. After your password, Margince shows **Set up two-step sign-in**.
Choose **Set up**. The steps are the same as on the **Account** page, from the picture to **Turn on**.
Keep your **Recovery codes**, choose **Done**, and Margince opens.

If you are already signed in when the switch goes on, you see this page the next time Margince checks.
**Sign out** leaves without setting it up.
Also called: required two-step sign-in, must set up, cannot continue.

### How do I sign in with two-step sign-in?
To sign in to Margince with two-step sign-in, enter your email and password as usual. Margince then shows **Verification code**.
Type the code your phone app shows now, or one of your backup codes, into **Code**, and choose **Verify code**.
Each backup code works once. A wrong code is refused, and after too many wrong codes Margince makes you wait a short while.

The code page stays open for five minutes. After that, choose **Back to sign in** and start again with your password.
Also called: sign-in code, code not working, use a backup code.

### How do I turn off two-step sign-in?
<!-- opens: settings/account -->
To turn off two-step sign-in in Margince, open Settings → **Account**, choose **Remove** in the **Two-step sign-in** row, and enter a code.
The code is one your phone app shows now, or one of your backup codes. Your backup codes stop working with it.
For new backup codes, or to move to a new phone, remove it and set it up again.

If your installation requires two-step sign-in, Margince asks you to set it up again straight away. That is also how you get new backup codes there.
Also called: new phone, new backup codes, lost phone.

Keep your backup codes where you can find them without your phone. If you lose both, an administrator can remove two-step sign-in for you from **Settings → Members**. You then sign in with your password and set it up again.

### How do I change my display name?
<!-- opens: settings/account -->
To change your display name in Margince, open Settings → **Account**, edit the **Display name** field, and choose **Save** or press Enter. **Save** shows once you change the name.
Your display name is what colleagues see on records you edit, in pickers and in the audit log.
It cannot be empty, and it can be up to 255 characters long. Your email address shows under it, and you do not change it here.
Also called: my name, profile name, rename myself.

### How do I add an email signature?
<!-- opens: settings/account -->
To add an email signature in Margince, open Settings → **Account** and choose **Edit signature** in the **Email signature** row.
1. Type your sign-off in **Sign-off**, in plain text.
2. Choose **Save**.

Margince adds the signature under every email you send yourself, above the unsubscribe footer. Leave it empty and your emails end with a short greeting and your name instead, in the message's language. When the message is too short to tell, that is the installation's language, then English. The row then says **No sign-off set**.

An email an agent sends carries no sign-off. AI drafts never add a sign-off, so this is the one that goes out.
Also called: email footer, sign-off, signature block.

### How do I change the language?
<!-- opens: settings/account -->
To change the language Margince uses with you, open Settings → **Account**. Pick one in the **Language** row: **English**, **Deutsch** or **Tiếng Việt**.
The screen changes at once. The choice is saved to your account, so your next sign-in uses it too, and it changes nothing for anyone else.
The sign-in page has its own language switch at the foot of the page.
Also called: change to German, switch to Vietnamese, locale, translation.

### How do I change the theme?
<!-- opens: settings/account -->
To change the theme in Margince, open Settings → **Account** and pick a choice in the **Appearance** row: **Light**, **Dark** or **System**.
You can do the same from the account menu at the top right, under **Theme**. Both change the same setting.
**System** follows the light or dark setting of your computer or phone, and keeps following it.
Also called: dark mode, light mode, appearance, colour scheme.

## Meetings

The Meetings page in Settings is where customers book time with you. It is a
stack of cards in this order: **Booking links**, **Calendar**,
**When people can book** and **What guests see**. While no Google or
Microsoft 365 calendar is connected, a card
**Connect a calendar to take bookings** comes first.

### How do I set up bookings?
<!-- opens: settings/meetings -->
To let customers book time with you in Margince, open Settings → **Meetings** and follow **Connect a calendar to take bookings**.
1. Choose **Connect calendar** and sign in with Google or Microsoft 365. You come back to Meetings, where a note says how the connection went.
2. Check your hours under **When people can book** and choose **Save availability**.
3. Under **Booking links**, turn on **Accept bookings** and choose **Copy link** to share it.
The setup card goes away once a calendar is connected.
Also called: booking page, scheduling link, book a meeting with me.

### How do I set my bookable hours?
<!-- opens: settings/meetings -->
To set when customers can book time with you in Margince, open Settings → **Meetings** and fill in **When people can book**.
1. Pick your **Working days**.
2. Set **From**, **Until** and your time zone. Guests see the hours in their own zone.
3. Under **Booking rules**, set **Minimum notice**, **Break between meetings** and **How far ahead**. Pick one of the given times, or **Other**.
4. Choose **Save availability**, which saves hours and rules together.
Saving works even while the calendar is being reconnected, because the rules never ask the calendar anything.
Also called: working hours, availability, office hours, notice.

Without the permission to book meetings, the page shows only the working hours.
You save them with **Save working hours**, and a note says "Booking meetings
needs permission from an administrator."

Opening meeting settings from a booking draft opens another tab; coming back loads the saved settings again.

### How do I change my time zone?
<!-- opens: settings/meetings -->
To change your own time zone in Margince, open Settings → **Meetings** and choose a place in the **Timezone** list under **When people can book**. Then choose **Save availability**, or **Save working hours** if you may not book meetings.
Your time zone is yours alone, and it decides when customers can book time with you.
The company's reporting time zone is a separate setting; see the next question.
Also called: change time zone, my time zone, local time.

### How do I change the company's reporting time zone?
<!-- opens: settings/company -->
To change the time zone reports use in Margince, open Settings → **Company profile** and set **Reporting timezone** in the **Installation** card.
Report periods use it for the whole team, and only Admin and Ops change it. Your own time zone on **Meetings** stays as it is.
Also called: company time zone, report time zone, installation time zone.

### How do I share a booking link?
<!-- opens: settings/meetings -->
To share your booking link in Margince, open Settings → **Meetings** and choose **Copy link** on the link in **Booking links**.
1. Turn on **Accept bookings**. It stays off until a calendar is connected and saved under **Calendar**.
2. Choose **Copy link** and paste it in any place, your email signature too.
Guests choose a time and enter their own details; you do not need to pick a contact first. With no link yet, the card offers **Create booking link**. You have one link today.
Also called: booking link, my calendar link, scheduling page.

The link row shows its name, how long it lasts, its video call and status: **Live**,
**Paused** or **No invitations**. Its menu holds three choices:

- **Edit link** opens a side panel. It holds **Meeting name** and how long it lasts
  (one of the given times or **Other**, 15 minutes to 8 hours). It also holds where the meeting
  happens: Google Meet or Microsoft Teams from the connected calendar, your own
  link or address, or no place. Last come the hours it uses, with a **Change
  hours** shortcut. Changes apply to new bookings only.
- **Preview public page** shows what guests will see.
- **Replace address** asks first, then gives the link a new address. Update any
  signature or page where you shared the old one. Meetings already booked keep
  their own change links.

You can always turn **Accept bookings** off: it pauses the link without
changing its address. Bookings may be on while the calendar has run out, lost
the right to write, or is gone. Then the card warns "Guests can book, but no
invitations go out" and offers **Reconnect calendar** or **Pause bookings**.

### Calendar

The **Calendar** card appears once a calendar is connected. It shows the
connected calendar as a row: it reads busy times, sends invitations, and shows
its last sync. Then comes **Add booked meetings to**, the calendar each booking
lands in. Last is **Also count as busy**, other calendars that hide time from
guests. Event names stay private.

With both Google and Microsoft calendars
connected, a **Calendar provider** choice comes first. **Save calendar** appears
only after a change.

A connection that has run out or can only read shows **Reconnect**, and keeps
the saved choices as they were. A calendar connected for reading only cannot
send invitations; reconnect it and allow invitation access.

### What guests see

**What guests see** puts a small preview of the public booking page beside
three rows:

- **Company name and logo** comes from the company profile, with **Change** or
  **Add logo**. With no company name set, guests see your name only.
- **Your name** comes from your account, with **Change**.
- **Reminder email** goes out one hour before each meeting, from your connected
  mailbox. So it needs a mailbox under Connections.

**Preview public page** opens the real page.

### How do I arrange a meeting with a contact?
Open the **Meetings** tab of the contact and choose **Book a meeting**. Suggest two or three times and check the email before you send it. Or share a personal booking link, or send an invitation for a time you already agreed.
Personal links run out, and can book one meeting.
Suggested times and personal links that still wait on a reply are listed on the same tab. **Withdraw** stops a link working before the guest uses it.

Your free times take private and internal busy time into account, without
copying those details into Margince. Margince checks bookable hours, notice and
free time around meetings when a guest books. **Creating your invitation…**
means it has not gone out yet. **Calendar invitation created** means the
calendar provider accepted it. The guest saying yes is a separate event. Open
the meeting from its line on the timeline to move it, cancel it, or try again
when sending it needs your help.

## Writing voice

The Writing voice page in Settings holds your **Voice DNA**: your personal
writing voice. It shapes drafts written for you, only you can see it, and it
learns only from samples you add.

### How do I set up my writing voice?
<!-- opens: settings/voice -->
To set up your writing voice in Margince, open Settings → **Writing voice**, add writing samples, then choose **Build Voice DNA**.
1. Under **Writing samples**, choose **Add writing samples** and drop or pick files, several at once.
2. The files can be `.txt`, `.md`, `.pdf`, `.docx`, `.vtt`, `.srt` or `.json`.
3. Add at least 800 words of your own writing. The bar shows how far you are.
4. Choose **Build Voice DNA**. It takes about 60 seconds, and goes on if you leave the page.

After the first build, the button reads **Rebuild Voice DNA**. Notes on how you want to sound go in **Preferences**, then **Save preferences**.
Also called: tone, writing style, Voice DNA, sound like me.

The best samples are emails you sent, then proposals and posts, then the written
record of calls or meetings. Leave out text others wrote, and AI drafts: Margince
learns a different voice from them. A read-only seat sees the page and cannot change it.

Margince reads a PDF or Word document to its text in the browser before it
sends anything. It skips a file with no text in it. It names a file it cannot
open (one with a password, or one that is broken), so you can copy its text in
instead.

When at least half of a file comes from named speakers, Margince reads it as a
conversation. The page asks which speaker is you, and keeps only your parts.
Under that share, the file counts as plain text and is taken whole. So an email
with a line that opens with a heading and a colon, such as "Frage: …", is not
asked about.

## AI access

The AI access page in Settings is where you connect your AI tool, such as
Claude or ChatGPT, to Margince. Under **For developers** you also make and take
back **passports**: the keys that let a script or your own code work as you.
Every member can open this page.

To connect your AI tool, see [How do I connect my AI tool to Margince?](agents-and-passports.md#how-do-i-connect-my-ai-tool-to-margince).

### How do I create an agent passport?
<!-- opens: settings/agents -->
To create an agent passport, open Settings → **AI access**, then **For developers**, and see [How do I give an agent access to Margince?](agents-and-passports.md#how-do-i-give-an-agent-access-to-margince).
Also called: API key, access token, connect an AI agent.

**Your connections** lists each AI tool you connected. Under **For developers**,
the **Agent rules** tab shows the list of actions agents may use. Choosing **Disconnect** on a
connection ends the whole connection, not one key. The
agent loses access on its next call and cannot get it back. Connecting it again
means approving access again.

See [Agents, passports and what they may
do](agents-and-passports.md#passports-how-an-agent-is-connected).

## Connections

The Connections page in Settings holds what *you* connected. That is your mail
and calendar account, your other email addresses, your overnight Morning brief
and your LinkedIn network. **Integrations** is the other side: what the *installation* is
connected to, such as webhooks and the company's Telegram bot.

The cards, in order:

- **Mail and calendar**: one row per account: Google, Microsoft 365, or a mail
  host connected over IMAP. What it brings in is listed under it: **New
  mail**, **Past email** and **Calendar**. Each account has a block **Who sees
  mail from {account}** with **Mail visibility**, a pointer to held threads in
  Capture activity, and **Advanced** settings. A row at the foot, marked
  **Whole company**, shows the company's email sharing rule.
- **Your other email addresses**: other names for your mailbox, a private domain
  or an address you forward from. Margince counts them as you, so mail between them is not
  captured and never creates a contact.
- **Overnight preparation**: whether Margince prepares your Morning brief
  overnight. When it is off, the card says **Off** and explains that the brief is
  not made before you start; nothing else stops working.
- **LinkedIn network**: import your **LinkedIn connections** from
  Connections.csv, set your LinkedIn profile URL, and see which companies on file
  you know someone at.

**What your connections found** counts what they brought in: **Senders**,
**Waiting on a decision** (only while a sender waits), **Held threads** and
**Network reach**. Choose **View** on a row to see its list.

### How do I connect my mailbox?
<!-- opens: settings/connections -->
To connect your mailbox and calendar to Margince, open Settings → **Connections** and choose **Connect Google** or **Connect Microsoft 365** in **Mail and calendar**. Once one account is connected, choose **Add account**.
1. **Add account** asks which account first: **Google**, **Microsoft 365** or **Another mail host**.
2. Under **What to bring in**, tick **Mail**, **Calendar** or both, and pick how much past email to import, or **None**. Choose **Continue**.
3. Sign in at Google or Microsoft and allow each permission it lists. With both ticked it asks twice: once for mail, then once for your calendar.
You land back on Connections, where a note says, for example, "Google connected" and "Mail and Calendar are syncing."
Also called: link my email, sync Gmail, sync Outlook, connect my calendar.

All the detail (IMAP settings, importing past mail, reconnecting, and what
capture files where) is in [Connecting your mailbox and calendar](connecting-mail-and-calendars.md#what-you-can-connect).

## Capture activity

The Capture activity page in Settings says what your connected mail turned into,
and holds the decisions you make about it. The cards, in order:

**Held threads.** Threads your mailbox is holding back from the team right now,
with the reason. **Share with the team** releases one; only you can release
yours.

**Capture exclusions** are addresses and domains whose messages never come into
the CRM. Your rules apply only to mailboxes you connected. The company's rules
apply to everyone, and only an administrator may add or remove one of those. A
rule applies from the next message; what is already captured stays.

**Capture activity** shows what the last 24 hours of your mail turned into.

**Outcomes** are counts for those 24 hours: **Captured**, **Internal**, **No
contact created**, **Awaiting check** and **Failed**. Click one to narrow the
list under it.

**Messages** is the log for each message: which step a message
stopped at, and why. Open it when a message you expected did not show up. By
default each row names the sender and a short subject. If your installation
turned this off, the page says so above the rows.

**Senders.** Every address your mailbox brought in, what was decided about
each, and your own answer where there is one. Only you see this list.

### Why did an email not show up in Margince?
To find out why an email did not show up in Margince, open Settings → **Capture activity** and look in **Messages** for the last 24 hours.
1. Click an **Outcomes** count, such as **Internal** or **No contact created**, to narrow the list.
2. Open a row's steps to see where that message stopped, and why.

Mail between colleagues is never stored, and a sender under **Capture exclusions** is always kept out.
Also called: missing email, email not captured, email not synced.
