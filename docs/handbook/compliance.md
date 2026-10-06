# Compliance: what you have to do, and what Margince does not check

**Margince checks none of the compliance paperwork below.** It will connect a
mailbox and read its mail whether or not you have done anything on this page.
There is no checkbox, no upload and no blocked step. German and Austrian law
expects these documents before an employer reads employee mailboxes, and holding
them is your company's obligation.

What Margince gives you is a way to check what the documents describe: the AI
egress reference says what leaves your installation, the Senders card says what
was decided, and the audit log says who read what.

### How do I make Margince GDPR-compliant before connecting mailboxes?
To prepare Margince for mail capture lawfully, do four things before anyone connects a work mailbox; Margince itself checks none of them.
1. Tell the colleagues whose mail will be read (Mitarbeiterinformation, Art. 13 GDPR).
2. Settle private use of work mail: consent per colleague, or a written ban (Einwilligung, §26(2) BDSG).
3. Agree it with the works council (Betriebsvereinbarung, §87(1) Nr. 6 BetrVG).
4. Record it for the regulator (Verarbeitungsverzeichnis und DSFA, Art. 30 and Art. 35).
Templates ship with the Margince documentation.
Also called: DSGVO, data protection, works council, Betriebsrat, DPIA.

### Does Margince check that we signed a works agreement?
No. Margince does not verify that you executed any compliance document, does not remind you, and does not stop working if you have not. Holding the documents is your company's obligation. The templates are not legal advice; whoever signs them must be able to answer for them.
Also called: compliance check, legal sign-off, is Margince compliant.

## The four compliance documents, in order

1. **Tell the colleagues whose mail will be read**: the
   *Mitarbeiterinformation* (Art. 13 GDPR). Before a mailbox is connected, not
   after. Keep the receipt: the *Empfangsbestätigung* shows you did it, which
   Art. 5(2) asks of you separately from doing it. Where there is no works
   council (Austria in particular), the individual agreement goes on that same
   sheet, because step 3 below has nobody to agree with.
2. **Settle private use**: the *Einwilligung* (§26(2) BDSG, Art. 7 GDPR). Where
   private use of the work mailbox is permitted or tolerated, the archive fills
   with correspondence from outside the company: a friend, a doctor, a landlord.
   The employment basis covers the employee. It does not cover anybody who
   merely wrote to them.

   So either **ban private use in writing and enforce it**, or collect the
   Einwilligung per colleague, per version. Prefer the ban. The consent covers
   the employee only: whether their private mail may be processed at all. It
   cannot cover those who write to them, because you will never reach the
   doctor to ask. Their data falls under the same legal basis and privacy notice
   as every other third party in captured mail. Keeping private mail out of the
   work mailbox is therefore the stronger choice.
3. **Agree it with the works council**: the *Betriebsvereinbarung* (§87(1) Nr. 6
   BetrVG). Mail capture is a system suitable for monitoring performance and
   conduct, so it is co-determined whether or not you intend to monitor anybody.
4. **Write it down for the regulator**: the *Verarbeitungsverzeichnis und DSFA*
   (Art. 30, Art. 35). One entry per processing operation.

The templates live in the compliance section of the Margince documentation. The
English set is for reading and internal circulation; the German versions are the
ones to execute.

## What Margince gives you to point at

| Question a regulator or a works council will ask | Where the answer is |
| --- | --- |
| Does mail leave our infrastructure? | The AI egress reference, which lists every AI task and whether its text can leave your installation |
| Who decided this message was private, and when? | The message's audience reason, and the audit log |
| What was decided about my correspondents? | Settings → Connections → Senders, per seat |
| Can an administrator read a held message? | No. An administrator cannot open a held message |
| What happens when the classifier is unavailable? | Everything stays held |

Margince cannot tell you whether your particular arrangement is lawful: the
templates have placeholders to fill in. For erasure, access requests, retention and consent inside Margince, see
[What is kept, what is destroyed](retention-exports-and-deletion.md).
