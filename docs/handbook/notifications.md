<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/notification-settings.tsx frontend/src/features/settings/notifications.queries.ts backend/internal/modules/notices/preference.go backend/internal/modules/notices/centre.go backend/internal/modules/notices/handlers.go frontend/src/features/settings/settings.tsx frontend/src/features/brief/briefdelivery.tsx -->
# Notifications

The **Notifications** page in Settings, under **You**, decides how each kind of notice reaches you. The choice is yours alone. No colleague, and no admin, can change it for you.

Every notice also stays in the notification center under the bell, whatever you choose here.

### Which kinds of notice can I set?
<!-- opens: settings/notifications -->
The page lists these kinds under **Delivery by kind**. Each row has a short note and a menu of choices.

| Kind | What it tells you | Standing choice |
|---|---|---|
| **Approvals waiting on you** | Somebody needs your decision before their work can go on. A colleague who named you on one of their team links counts too. | **Email** |
| **Project health set by the AI** | The AI marked a project of yours at risk, off track or back on track. | **In the app** |
| **Automations that ran** | A rule your company set fired, or could not. | **In the app** |
| **Leads past their deadline** | A lead nobody has answered inside the time your team agreed. | **In the app** |
| **Mail capture** | A mailbox of yours cannot be read, or its mail has stopped coming in. | **In the app** |
| **System notices** | A time when the system is down, a limit reached, and anything else the installation has to tell you. | **In the app** |
| **Nudges from colleagues** | A teammate pointing you to a deal, a reply or work of yours that is waiting. | **In the app** |

The standing choice is what you get until you pick one. It follows the installation, so it can change for you until you choose. A choice you make stays as you made it.

### What do the choices mean?
The menu offers up to four choices:
- **Off**: no notice of this kind reaches you, except in the notification center.
- **In the app**: the notice waits where you already look.
- **Email**: Margince sends the notice as an email.
- **Daily digest**: the notices of this kind come in your daily digest.

Not every row offers every choice. Margince leaves out a choice it cannot keep:
- **Email** is on the menu for **Approvals waiting on you** only. No other kind sends a single email.
- **Daily digest** is on the menu for every kind.
- **Nudges from colleagues** has no **Off**. A colleague's words always reach you.

### What if this installation sends no email?
Then the page says "Email is not set up" above the rows. **Email**, **Daily digest** and the **Email summaries** reach no mailbox, and every notice still shows in the app. A row offers neither **Email** nor **Daily digest**, unless it already holds one. An admin also gets **How to set up email**.

### How do I save a change?
Pick a choice on one or more rows. Then choose **Save** in the bar at the foot of the page.
**Discard** puts every row back as it was.
Margince saves the rows one at a time. If it refuses one, the page says "“Approvals waiting on you” was not saved", with the name of the row it refused. Rows after it stay as you left them, and **Save** tries again.

### What is the Browser notifications row?
The first row, **Browser notifications**, says whether your browser may show a due task reminder while Margince is open in a tab.
- Until you decide, the row offers **Allow**. Press it, then answer the browser's own question.
- Once allowed, the row says **On**. To turn it off, block notifications for this site in your browser settings.
- If the browser blocks them, the row says so and offers no **Allow**.
- If the browser cannot show notifications, the row says that. A due reminder still leads your Home and your Worklist.

This row needs no **Save**. Your browser holds the answer.

### Where are the morning brief and weekly review settings?
They sit lower on the same page, under **Email summaries**. See [Your own settings](your-own-settings.md#how-do-i-change-my-notifications) for each row.

Also called: email notifications, turn off emails, alerts, daily digest, browser notifications, email not set up.
