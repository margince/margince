<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/settings.jurisdiction.tsx frontend/src/features/capture/capture-senders.tsx backend/internal/shared/ports/compliancedocs backend/internal/shared/ports/jurisdiction backend/internal/modules/consent/dsrdeadline.go backend/internal/modules/capture/purgefloor.go backend/internal/modules/identity/settingsindividualperformance.go frontend/src/features/sign-in/installation-settings.individual.tsx -->
# Compliance: what you have to do, and what Margince does not check

**Margince checks none of the compliance papers below.** It will connect a
mailbox and read its mail whether or not you have done anything on this page.
There is no box to tick, no upload and no blocked step. German and Austrian law
expects these papers before a company reads the mailboxes of its own team, and
holding them is your company's duty.

What Margince gives you is a way to check what the papers describe. The AI
egress reference says what leaves your installation. The Senders card says what
was decided, and the audit log says who read what.

### How do I make Margince GDPR compliant before connecting mailboxes?
To get Margince ready for mail capture within the law, do four things before anyone connects a work mailbox; Margince itself checks none of them.
1. Tell the colleagues whose mail will be read (Beschäftigteninformation, Art. 13 GDPR).
2. Settle private use of work mail: consent per colleague, or a written ban (Einwilligung, §26(2) BDSG).
3. Agree it with the works council (Betriebsvereinbarung, §87(1) Nr. 6 BetrVG).
4. Record it for the regulator (Verarbeitungsverzeichnis und DSFA, Art. 30 and Art. 35).
With Germany chosen as the law, **Settings → Governance → Jurisdiction** lists the templates.
Also called: DSGVO, data protection, works council, Betriebsrat, DPIA.

### Does Margince check that we signed a works agreement?
No. Margince does not check that you signed any compliance paper, does not bring it up again, and does not stop working if you have not. Holding the papers is your company's duty. The templates are not legal advice; whoever signs them must be able to answer for them.
Also called: compliance check, legal sign-off, is Margince compliant.

### Can we stop Margince from showing each colleague's performance?
Yes. Admin and Ops switch off **Show individual performance** on the **Installation** card under **Settings → Company**. The audit log records each change. While it is off:
- Reports, analytics questions, forecasts, forecast shares, targets and editions show no colleague's figures. That holds for their exports and for agent answers about them too.
- Splitting or filtering figures by owner or meeting host is refused, and so is a view of one colleague.
- A figure that holds a colleague's work shows only if it holds the work of at least two colleagues besides you who still work here.
- Team weeks are not made, sent or opened. Each member's own week stays.
- Everyone still sees their own figures.

What the switch does not cover yet:
- the **Team** board on Home and its coaching moves;
- a lead's view of a named member's queue;
- the team section of the morning mail;
- team weekly plans.

What the switch cannot promise:
- Records stay open to read. A list of deals still shows each deal's owner and amount.
- Totals of teams that share members can be compared. Say you share one team with A and B, one with A and C, and one with B and C. The three totals together give each colleague's figure.

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

With Germany chosen as the law, the templates are on **Settings → Governance →
Jurisdiction**, to read and download. The English set is for reading and for
passing around inside the company; the German versions are the ones to sign.

## What Margince gives you to point at

| Question a regulator or a works council will ask | Where the answer is |
| --- | --- |
| Does mail leave the company's systems? | The AI egress reference, which lists every AI task and whether its text can leave your installation |
| Who decided this message was private, and when? | The message's audience reason, and the audit log |
| What was decided about the senders who write to me? | Settings → Capture activity → Senders, per seat |
| Can an administrator read a held message? | No. An administrator cannot open a held message |
| What happens when the sorting model is down? | Everything stays held |
| Can a manager see each colleague's sales figures? | Not in reports and forecasts while **Show individual performance** is off. Each member sees their own figures. The compliance answer above lists what the switch does not cover |

Margince cannot tell you whether your own way of working is within the law: the
templates have blanks to fill in. For erasure, access requests, retention and consent inside Margince, see
[What is kept, what is destroyed](retention-exports-and-deletion.md).

## Which law applies

Margince follows one law for the whole installation. An admin chooses it at first setup, or later on **Settings → Governance → Jurisdiction**. The page shows what the choice changes:

- how long business mail is kept before anyone may delete it;
- the due date of a privacy request;
- the rules for sales mail.

With no law chosen, a privacy request is due in one calendar month. Mail is then kept for the longest time any built-in law asks for.

A change to a law that keeps mail for a shorter time does not cut that time for mail you already hold. That mail keeps the old time, and the page says until when.
