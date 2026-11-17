<!-- prose:plain -->
<!-- describes: frontend/src/app/trialpill.tsx frontend/src/app/trialring.tsx frontend/src/app/licensebanner.tsx frontend/src/app/trialended.tsx frontend/src/components/trialcontact.tsx backend/internal/compose/installtrial/installtrial.go backend/internal/modules/identity/triallock.go -->
# The trial

A Margince you set up yourself runs for 14 days with no licence. The days count from the day its
first workspace was made, even if that workspace is archived now. While they run, everything works
and seats have no limit.

Members who may read the licence see how many days are left. Admin and Ops hold that right by
default. Short lines round the agent in the sidebar show one mark for each day. A small label under
the agent says how many days are left. **Settings → Seats and license** says it too.

In the last week the marks change colour and a notice shows at the top of every page. You can hide
it until the next day, but not on the last day.

### How do I get a licence or more time?
Press the label under the agent, or **Talk to the Margince team** in the notice. Pick what
would help (**More time**, **A license** or **A short call**) and add a line if you like. Margince
writes the message for you: it names your company, the day the trial ends and how many seats are in
use.

**Open the contact form** opens the form on the Margince website, where you can paste it.
**Copy text** copies it, and the team's address is shown to write to from any mail program.
**Open in my mail app** opens it in your mail app if one is set up.
Also called: free trial, test licence, extend the trial, get a licence, contact sales.

### What happens when the trial ends?
Margince stops working until it has a licence. There is no grace period. Every member sees **Your
Margince trial has ended** in place of the app, with the same button to write to the team, and
**Sign out**.

Nothing is deleted. Until the licence is in place, Margince refuses changes to your records. Some
things still work: reading, data exports, privacy requests, the licence itself, and each member's
own password, two-step sign-in and sessions. The screen has no way to reach an export, so ask
whoever runs your Margince to make one for you.

How the licence goes in depends on where your server admin puts the key:

- **In a licence file** (`license.token_file`, or a `${file:…}` value in `license.token`): an
  admin presses **Check now**. It is on the same screen and in **Settings → Seats and license**.
  Margince opens again at once.
- **Typed into the configuration** (`license.token` or `MARGINCE_LICENSE`): the admin starts
  Margince again, because that value is read only when Margince starts.

Background work starts again on its own within about 15 minutes. An agent you connected stops
too: every call it makes is refused. Margince sends no email until then, and that includes privacy notices
and confirmation links.
Also called: trial ended, trial expired, Margince stopped working, locked.

### What happens when a licence ends?
When your licence passes its end date, or its key is not valid, Margince locks in the same way.
There is no grace period. **Settings → Seats and license** warns you 90 days before the end date.
Every member sees **Your Margince license is no longer valid** in place of the app, with the same
button to write to the team. Nothing is deleted, and the new licence goes in as described above.
Also called: licence expired, licence ended, licence not valid, renew the licence.
