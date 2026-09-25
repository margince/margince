# Compliance: what you have to do, and what Margince does not check

**Margince checks none of the compliance paperwork below.** The product will
connect a mailbox and read its mail whether or not you have done anything on
this page. There is no checkbox, no upload, no blocked flow. The documents below
exist because German and Austrian law expects them before an employer reads
employee mailboxes, and holding them is the customer's obligation, not the
software's.

That is a deliberate decision rather than an omission. A product that refused to
run until it was shown a signed document would be checking a box, not a
practice: it cannot tell a real works agreement from a scanned blank page, and a
customer who satisfied the checkbox would reasonably believe they were covered.
What Margince does instead is make the facts checkable: the AI egress table says
what leaves the machine, the Senders card says what was decided, and the audit
log says who read what, so the documents you sign describe something you can
verify.

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
   after. Keep the receipt: the *Empfangsbestätigung* is what shows you did it,
   which Art. 5(2) asks of you separately from doing it. Where there is no works
   council — Austria in particular — that same sheet is where the individual
   agreement goes, because step 3 below has nothing to agree with.
2. **Settle private use**: the *Einwilligung* (§26(2) BDSG, Art. 7 GDPR). Where
   private use of the work mailbox is permitted or tolerated, the archive fills
   with correspondence from outside the company entirely — a friend, a doctor, a
   landlord. The employment basis reaches the employee. It reaches nobody who
   merely wrote to them.

   So: **ban private use in writing and enforce it**, or collect the
   Einwilligung, per colleague, per version. Prefer the ban, and know what the
   consent does and does not do. It is the employee's, and it settles the
   employee's half — whether their private mail may be processed at all. It is
   not their correspondents' consent and cannot be: you will never reach the
   doctor to ask. Their data rides the same basis and the same privacy notice
   as every other third party in captured mail, which is why keeping private
   correspondence out of the mailbox in the first place is the stronger
   position rather than merely the cheaper one.
3. **Agree it with the works council**: the *Betriebsvereinbarung* (§87(1) Nr. 6
   BetrVG). Mail capture is a system suitable for monitoring performance and
   conduct, so it is co-determined whether or not you intend to monitor anybody.
4. **Write it down for the regulator**: the *Verarbeitungsverzeichnis und DSFA*
   (Art. 30, Art. 35). One entry per processing operation, each naming the code
   path that enforces it.

The templates live in the compliance section of the Margince documentation. The
English set is for reading and internal circulation; the German versions are the
ones to execute.

## What Margince gives you to point at

| Question a regulator or a works council will ask | Where the answer is |
| --- | --- |
| Does mail leave our infrastructure? | The AI egress reference, generated from the model routing table |
| Who decided this message was private, and when? | The message's audience reason, and the audit log |
| What was decided about my correspondents? | Settings → Connections → Senders, per seat |
| Can an administrator read a held message? | No. The audience gate has no admin arm |
| What happens when the classifier is unavailable? | Everything stays held |

## What none of this covers

Margince does not verify that you executed any of it, does not remind you, and
will not stop working if you have not. It also cannot tell you whether your
particular arrangement is lawful: these are templates with placeholders, not
advice, and whoever signs them needs to be somebody who can answer for them.

For erasure, access requests, retention and consent inside Margince, see
[What is kept, what is destroyed](retention-exports-and-deletion.md).
