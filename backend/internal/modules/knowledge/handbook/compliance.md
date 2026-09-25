# Compliance: what you have to do, and what Margince does not check

**Margince checks none of this.** The product will connect a mailbox and read
its mail whether or not you have done anything on this page. There is no
checkbox, no upload, no blocked flow — the documents below exist because German
and Austrian law expects them before an employer reads employee mailboxes, and
holding them is the customer's obligation, not the software's.

That is a deliberate decision rather than an omission. A product that refused to
run until it was shown a signed document would be checking a box, not a
practice: it cannot tell a real works agreement from a scanned blank page, and a
customer who satisfied the checkbox would reasonably believe they were covered.
What the product does instead is make the facts checkable — the egress table
says what leaves the machine, the Senders page says what was decided, and the
audit log says who read what — so the documents you sign describe something you
can verify.

## In order

1. **Tell the colleagues whose mail will be read** —
   [Mitarbeiterinformation](../compliance/de/mitarbeiterinformation.md)
   (Art. 13 GDPR). Before a mailbox is connected, not after. Keep the receipt:
   [Empfangsbestätigung](../compliance/de/empfangsbestaetigung.md) is what shows
   you did it, which Art. 5(2) asks of you separately from doing it. Where there
   is no works council — Austria in particular — that same sheet is where the
   individual agreement goes, because step 3 below has nothing to agree with.
2. **Settle private use** —
   [Einwilligung](../compliance/de/einwilligung-email-erfassung.md)
   (§26(2) BDSG, Art. 7 GDPR). Where private use of the work mailbox is
   permitted or tolerated, the archive fills with correspondence from outside
   the company entirely — a friend, a doctor, a landlord. The employment basis
   reaches the employee. It reaches nobody who merely wrote to them.

   So: **ban private use in writing and enforce it**, or collect the
   Einwilligung, per colleague, per version. Prefer the ban, and know what the
   consent does and does not do. It is the employee's, and it settles the
   employee's half — whether their private mail may be processed at all. It is
   not their correspondents' consent and cannot be: you will never reach the
   doctor to ask. Their data rides the same basis and the same privacy notice
   as every other third party in captured mail, which is why keeping private
   correspondence out of the mailbox in the first place is the stronger
   position rather than merely the cheaper one.
3. **Agree it with the works council** —
   [Betriebsvereinbarung](../compliance/de/betriebsvereinbarung-vorlage.md)
   (§87(1) Nr. 6 BetrVG). Mail capture is a system suitable for monitoring
   performance and conduct, so it is co-determined whether or not you intend to
   monitor anybody.
4. **Write it down for the regulator** —
   [Verarbeitungsverzeichnis und DSFA](../compliance/de/verarbeitungsverzeichnis-und-dsfa.md)
   (Art. 30, Art. 35). One entry per processing operation, each naming the code
   path that enforces it.

The `en/` folder holds the same four documents in English. They are for reading
and for internal circulation; the German versions are the ones to execute.

## What the product gives you to point at

| Question a regulator or a works council will ask | Where the answer is |
| --- | --- |
| Does mail leave our infrastructure? | [ai-egress.md](../reference/ai-egress.md), generated from the routing table |
| Who decided this message was private, and when? | The message's audience reason, and `audit_log` |
| What was decided about my correspondents? | Settings → Connections → Senders, per seat |
| Can an administrator read a held message? | No. The audience gate has no admin arm |
| What happens when the classifier is unavailable? | Everything stays held. See the same page |

## What none of this covers

Margince does not verify that you executed any of it, does not remind you, and
will not stop working if you have not. It also cannot tell you whether your
particular arrangement is lawful: these are templates with placeholders, not
advice, and whoever signs them needs to be somebody who can answer for
them.
