<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/settings.jurisdiction.tsx frontend/src/features/settings/controllerparticulars.tsx frontend/src/features/capture/capture-senders.tsx backend/internal/shared/ports/compliancedocs backend/internal/shared/ports/jurisdiction backend/internal/modules/consent/dsrdeadline.go backend/internal/modules/consent/ruleselectors.go backend/internal/modules/consent/optoutack.go backend/internal/modules/consent/optoutrecords.go backend/internal/modules/consent/recipientpack.go backend/internal/modules/consent/draftcontent.go frontend/src/components/lawfulbasiswarning.tsx frontend/src/features/mail/composecountry.tsx backend/internal/modules/consent/requesttask.go backend/internal/modules/capture/purgefloor.go backend/internal/modules/identity/settingsindividualperformance.go frontend/src/features/sign-in/installation-settings.individual.tsx -->
# Compliance: what you have to do, and what Margince does not check

**Margince checks none of the compliance papers below.** It will connect a
mailbox and read its mail whether or not you have done anything on this page.
There is no box to tick, no upload and no blocked step. German and Austrian law
expects these papers before a company reads the mailboxes of its own team, and
holding them is your company's duty.

What Margince gives you is a way to check what the papers describe. The AI
egress reference says what leaves your installation. The Sender decisions card says what
was decided, and the audit log says who read what.

### How do I make Margince GDPR compliant before connecting mailboxes?
To get Margince ready for mail capture within the law, do four things before anyone connects a work mailbox; Margince itself checks none of them.
1. Tell the colleagues whose mail will be read (Beschäftigteninformation, Art. 13 GDPR).
2. Settle private use of work mail: consent per colleague, or a written ban (Einwilligung, §26(2) BDSG).
3. Agree it with the works council (Betriebsvereinbarung, §87(1) Nr. 6 BetrVG).
4. Record it for the regulator (Verarbeitungsverzeichnis und DSFA, Art. 30 and Art. 35).
With Germany chosen as the law, **Settings → Compliance → Jurisdiction** lists the templates.
Also called: DSGVO, data protection, works council, Betriebsrat, DPIA.

### Does Margince check that we signed a works agreement?
No. Margince does not check that you signed any compliance paper, does not bring it up again, and does not stop working if you have not. Holding the papers is your company's duty. The templates are not legal advice; whoever signs them must be able to answer for them.
Also called: compliance check, legal sign-off, is Margince compliant.

### Can we stop Margince from showing each colleague's performance?
Yes. Admin and Ops switch off **Show individual performance** on the **Forecast** tab of **Settings → Deals**. The audit log records each change. While it is off:
- Reports, analytics questions, forecasts, forecast shares, targets and editions show no colleague's figures. That holds for their exports and for agent answers about them too.
- Splitting or filtering figures by owner or meeting host is refused, and so is a view of one colleague.
- A figure that holds a colleague's work shows only if it holds the work of at least two colleagues besides you who still work here.
- The same holds for the difference between two totals. A team total is hidden when taking away the members of another team you can see would leave fewer than two colleagues.
- Say one team is A, B and C, and a second team is A and B. The first team is hidden, because it minus the second is C alone. The second team still shows. The difference counts members, so a total that would show that C won nothing is hidden too.
- A company total is checked by who added to it, not by who is on a team. A colleague on no team who added nothing hides nothing. A colleague on no team who did add to it hides it when all other figures behind it come from one team you can see.
- Team weeks are not made, sent or opened. Each member's own week stays.
- Everyone still sees their own figures.

What the switch does not cover yet:
- the **Team** board on Home and its coaching moves;
- a lead's view of a named member's queue;
- the team section of the morning mail;
- team weekly plans.

What the switch cannot promise:
- Records stay open to read. A list of deals still shows each deal's owner and amount.
- One team's total can be compared over time. When a member joins or leaves, this month's total minus last month's can show that member's figure.
- Totals are compared two at a time. Several teams that together cover all but one colleague of a total are not taken away together, apart from your own **My teams**. Their totals can still add up to that colleague's figure.
- A company total that holds no figure of a colleague on no team still shows, so it can show that this colleague added nothing.

Also called: works council, Betriebsrat, figures per rep, team figures.

## The four compliance documents, in order

1. **Tell the colleagues whose mail will be read**: the
   *Beschäftigteninformation* (Art. 13 GDPR). Do it before a mailbox is
   connected, not after. Keep the receipt: the *Empfangsbestätigung* shows you
   did it, which Art. 5(2) asks of you on its own. Where there is no works
   council (most often in Austria), add each colleague's written agreement to
   that same sheet. The receipt alone is not that agreement. Step 3 below then
   has nobody to agree with.
2. **Settle private use**: the *Einwilligung* (§26(2) BDSG, Art. 7 GDPR). Private use of
   the work mailbox may be allowed or put up with. Then the archive fills with
   mail from outside the company: a friend, a doctor. The legal basis for work
   mail covers the colleague. It does not cover anybody who only
   wrote to them.

   So either **ban private use in writing**, and hold to it, or get the
   Einwilligung from each colleague, per version. Choose the ban. The consent covers
   the colleague only: whether their private mail may be used at all. It
   cannot cover those who write to them, because you will never reach the
   doctor to ask. Their data falls under the same legal basis and privacy notice
   as every other outsider in captured mail. Keeping private mail out of
   the work mailbox is the choice to make.
3. **Agree it with the works council**: the *Betriebsvereinbarung* (§87(1) Nr. 6
   BetrVG). Mail capture is a system able to watch how the team works and acts.
   So the works council must agree, even if you never mean to watch anybody.
4. **Write it down for the regulator**: the *Verarbeitungsverzeichnis und DSFA*
   (Art. 30, Art. 35). One entry per processing operation.

With Germany chosen as the law, the templates are on **Settings → Compliance →
Jurisdiction**, to read and download. The English set is for reading and for
passing around inside the company; the German versions are the ones to sign.

## What Margince gives you to point at

| Question a regulator or a works council will ask | Where the answer is |
| --- | --- |
| Does mail leave the company's systems? | The AI egress reference, which lists every AI task and whether its text can leave your installation |
| Who decided this message was private, and when? | The message's audience reason, and the audit log |
| Does the AI read private mail? | Only to find a request owed an answer, and only mail private by a routine setting. The AI never reads mail marked confidential or held by hand. It also skips mail held because of who it is with, or because only colleagues are on it. It waits until a thread's own privacy check is done. **Check private mail for requests**, under **Settings → AI models → Tasks**, turns this off; it is on at installation. Only a signed-in admin changes it, never an AI agent |
| What was decided about the senders who write to me? | Settings → Connections → Activity → Sender decisions, per seat |
| Can an administrator read a held message? | No. An administrator cannot open a held message |
| What happens when the sorting model is down? | Everything stays held |
| Can we show an authority a contact's opt-out history? | On the contact's **Research** tab, **Download opt-out records** in the consent panel: every consent given or withdrawn, every stop with when the refusal arrived and when sending stopped, and every opt-out receipt. Under Vietnamese law no retention rule removes them within a year |
| Can a manager see each colleague's sales figures? | Not in reports and forecasts while **Show individual performance** is off. Each member sees their own figures. The compliance answer above lists what the switch does not cover |

Margince cannot tell you whether your own way of working is within the law: the
templates have blanks to fill in. For erasure, access requests, retention and consent inside Margince, see
[What is kept, what is destroyed](retention-exports-and-deletion.md).

## Which law applies

Margince follows one law for the whole installation. An admin chooses it at first setup, or later on **Settings → Compliance → Jurisdiction**. The page shows what the choice changes:

- how long business mail is kept before anyone may delete it;
- the due date of a privacy request;
- who your company says it is, and whom to ask about data, at the foot of its mail;
- the sales-mail rules for recipients under that law.

The sales-mail rules follow the law where each recipient is, not where you are. Margince reads it from the contact's address, then from the address of the company they work for. For a lead, it reads the address of the lead's company. If it is wrong, fix the address. With no address on record, sales mail they did not ask for needs their consent. Every such mail offers a way to unsubscribe.

To correct it for one message, click the globe on the recipient in the composer. It shows the country Margince read and where it read it. You can pick another country or clear it for this message only; the contact's record does not change. A cleared country means no country's sales-mail rules apply, but consent is still needed and an objection still stops the mail. Your company's own duties at the foot of the mail stay. The decision record and the audit log name the country you set. Only you can set it: an agent cannot.

Some laws change what the mails to a contact about their own data may ask:

- Under German law, the mail that asks a contact to check their details asks nothing about news.
- Under Vietnamese law, Margince sends the mail that asks a contact to confirm a subscription only when the contact asked for it first. Log the request on the contact first: their call, the form they sent, or their mail.

When you create a contact by hand, say where it came from. The contact's privacy page shows your answer. It says the source is unknown only when you leave the answer empty.

Under Vietnamese law, a contact who refuses marketing email gets a receipt at once. It states when the refusal reached you and when sending stopped, in the contact's local time, and carries no "Powered by Margince" line. A refusal can come by phone or mail. Then tell your agent when it reached you. The receipt prints that time, not the time it was recorded. A receipt not sent within an hour shows in **Settings → System health** under **Opt-out receipts**.

Vietnamese data-protection law also follows the recipient. When a contact's or lead's address is a Vietnamese one, the Vietnamese rules apply to them whatever law you chose:

- the privacy notice, the record confirmation and the privacy page are written in Vietnamese, with the Vietnamese rights text;
- the foot of their mail says who your company is and whom to ask about data;
- a request to fix their details is due in 10 days, to delete them in 20 days, and to stop using them in 15 days. Margince makes a task for each request, due on that date;
- the first answer to a request is due in 2 working days, counted in Vietnamese time from Monday to Friday;
- when they object to the use of their data or take back their consent, Margince makes a task due in 15 days;
- a request to stop using their data, or to take back consent to anything other than marketing, also gets an email at once. It says when the request arrived and when it will be done;
- the reason recorded for using their data is their consent to that use, with its date. Your own business need is never enough;
- without that consent, the send screen and the approval card show the warning **No valid basis recorded**. You can still send once you confirm it with **Send anyway**. The audit log records your confirmation, and one confirmation covers one send. A send nobody confirmed is held for a colleague to decide;
- **Ask for consent** in that warning sends the contact the mail that asks for their consent, and does not send your message. It shows when a consent purpose covers this kind of mail, as the built-in **Business correspondence** purpose covers replies and other business mail. Once the contact confirms, the warning no longer shows for them. Sales mail is different: without the contact's consent it goes out only as one message a colleague sends with a recorded reason.

Margince learns where a contact is only from the addresses above. A Vietnamese contact whose address is not a Vietnamese one is not found. The Vietnamese rules do not apply to that contact.

With no law chosen, a privacy request is due in one calendar month. Mail is then kept for the longest time any built-in law asks for.

The **Company details in mail** card on the same page holds what your mail says about your company. Its fields are **Legal name**, **Postal address**, **Data protection contact**, **How to refuse advertising** and **Advertiser contact**.

Where a law in use asks for its own language, the card shows the same fields in that language. Mail and pages in that language show this text where you fill it in.

Under Vietnamese law, **How to refuse advertising** must name a web page or social network link, an email address and a phone number. Saving without one of them shows which part is missing. Sales mail to a recipient under Vietnamese law goes out only once **How to refuse advertising** and **Advertiser contact** are filled in, in Vietnamese.

While you write sales mail to a recipient under Vietnamese law, notes above the send button say what the mail does not meet. None of them stops the send:

- The mail does not start in Vietnamese.
- The mail claims to be the best, the leading or the only one, in Vietnamese or in English, and names no proof. Name the proving document with its number and date in the mail.
- Your company is not set up under Vietnamese law. A company from outside that does not work there must use a Vietnamese business whose service is sending sales mail for others. Your company has to see to this.

Vietnamese law does not ask for sales mail to go out from a Vietnamese mail server. If you run your own, ask whoever runs Margince to use a Vietnamese mail server where there is one.

How long business mail is kept follows the legal entity of the colleague it belongs to. That is the colleague who sent it, or whose mailbox it arrived in. Mail that names no colleague follows the owner of the contact or deal it is filed under. A legal entity that follows another law keeps its colleagues' mail under that law, but never for a shorter time than the law chosen here. Mail that belongs to nobody uses the law chosen here. When the colleagues of one mail work under two different laws, the mail is kept for the longer time.

When a colleague moves to another legal entity, or a legal entity changes its law, mail already kept keeps its time.

A change to a law that keeps mail for a shorter time does not cut that time for mail you already hold. That mail keeps the old time, and the page says until when.
