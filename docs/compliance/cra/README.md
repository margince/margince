# Reporting an actively exploited vulnerability (CRA Article 14)

This runbook covers Gradion's duty as manufacturer under CRA Article 14. The
customer-side documents in [`../en/`](../en/README.md) are what an installation
signs before it reads employee mail. This directory is what we do when a
weakness in Margince is being exploited in the wild.

The obligation is Article 14 of Regulation (EU) 2024/2847 (the Cyber
Resilience Act). Two dates, and they do different jobs:

- **11 September 2026**: when Article 14 starts applying. It is in force now.
- **11 December 2027**: the transitional cutoff. Article 69(3) extends the
  Article 14 reporting duty to in-scope products placed on the market before
  that date, so a product older than the regulation's substantive requirements
  still carries the duty to report.

The regulation governs. This page is the operational reading of it, written so
nobody has to do that reading with a 24-hour clock already running.

## The three cases, and which one you are in

Decide this first, because two of the three carry no clock for us.

| What happened | Whose duty | Our clock |
| --- | --- | --- |
| A vulnerability in Margince is being actively exploited | ours, as manufacturer | yes: start it now |
| A customer's installation was breached through something that is not a Margince weakness (a stolen credential, their own infrastructure) | theirs, as operator | none |
| A Margince vulnerability is being exploited at a live installation | both, together | yes, and see the warning below |

In the overlap case the two duties fire at once and they point in opposite
directions. Ours is to file. Theirs is to patch and, where their own law says
so, to report. Notify the operators that they must patch, and do not name any
installation, customer or affected party in the filing. The authority is
told about the product and the exploitation, and nothing about who was hit.

## When the clock starts

The clock starts when we **become aware**. That is the moment a maintainer
holds a credible report that a weakness in Margince is being exploited. It is
earlier than confirmation, reproduction, triage or understanding.

A report that arrives at 23:00 on a Friday starts the clock at 23:00 on that
Friday. There is no business-hours clause in Article 14 and this runbook does
not invent one.

Write the awareness timestamp down first. All three reports are measured from
it.

## The three reports

Each one is a separate filing with its own deadline, measured from awareness
(T₀). All three are filled skeletons in this directory: open the file, fill
the bracketed slots, file it.

| # | Report | Deadline | Template |
| --- | --- | --- | --- |
| 1 | Early warning | **T₀ + 24 hours** | [early-warning-24h.md](early-warning-24h.md) |
| 2 | Vulnerability notification | **T₀ + 72 hours** | [vulnerability-report-72h.md](vulnerability-report-72h.md) |
| 3 | Final report | **14 days** after a corrective or mitigating measure is available | [final-report.md](final-report.md) |

The first two are due whether or not the vulnerability is understood by then.
An early warning that says "we know it is being exploited and little else" is
the report Article 14 asks for at 24 hours. A late report is late however
complete it is.

A severe incident affecting the security of the product follows the same
24-hour and 72-hour rhythm, and its final report is due one month after
the 72-hour notification rather than 14 days. Everything else on this page
applies unchanged.

## Where it goes

Two recipients, at the same time: the CSIRT designated as coordinator in the
member state of our main establishment, and ENISA. Article 16 establishes a
single reporting platform through which both are reached, so in practice this
is one submission with two addressees.

Article 14(7) gives a rule for which coordinator applies. It names the CSIRT of
the member state where the manufacturer's product-cybersecurity decisions are
predominantly taken. Where that member state cannot be determined, the chain
continues with the EU establishment with the most employees. For a manufacturer
with no EU main establishment, it then runs through the authorised
representative, the importer, the distributor, and last the member states where
users are.

On the first limb ours is Germany, because the decisions about this product's
security are taken where it is built. So the coordinator is **BSI** (Bundesamt
für Sicherheit in der Informationstechnik), whose operational arm is CERT-Bund.
Its published reporting channel is on `bsi.bund.de`. The submission address is
recorded in the pre-registration block below, with the date it was checked.

Re-decide the coordinator when the basis changes. A second establishment, a
move of where security decisions are taken, or an EU presence arriving or
leaving changes which CSIRT is the right recipient. A notification sent to the
wrong one does not count as made. The pre-registration block below records the
basis beside the address, so the two are re-checked together.

## And tell the operators

Separately from the filing, and without undue delay once we are aware, tell
whoever runs a Margince installation three things: there is an actively
exploited vulnerability, what mitigates it, and what corrective measure is
coming or available. Article 14(8) puts that duty on the manufacturer, and
filing does not discharge it. Where we fail to do it in time, the notified
CSIRT may do it for us.

**The owner is whoever fixed T₀.** The maintainer who started the clock owns
the operator notice until it is sent. This stops two maintainers each assuming
the other told the customers.

**The first channel** is the GitHub Security Advisory the private report
already lives in ([SECURITY.md](../../../SECURITY.md)). Publish it once
publishing helps operators more than it helps an attacker. In an actively
exploited case that point arrives early, because the exploit is already in
use. Publishing also puts the vulnerability where automation can read it, which
is the machine-readable form Article 14(8) asks for where appropriate.

An advisory reaches whoever watches this repository, and that is a different
set from whoever runs the software. A self-hosted installation subscribes to
nothing by default. So the advisory is the minimum: every operator we hold a
contact route for is told directly, by that route. The notice carries the
risk, the mitigation available now, and the corrective measure.

Where no route exists for an installation, the duty is still open. Record the
gap in the final report's *users informed* block.

## Before any of this happens

These cannot be done under a running clock. They are done once and checked
when they change.

- [ ] **The reporting account exists.** Registered on the Article 16 single
      reporting platform, with the submission route and the credential holder
      recorded here, and at least one other maintainer able to reach it.
- [ ] **The coordinator's channel is confirmed** against BSI's own published
      contact, and the date of that check is written next to it. An authority's
      address can change.
- [ ] **The Article 14(7) basis is recorded:** which limb of the rule selects
      our coordinator, and why. Re-check it whenever an establishment changes,
      or where product-security decisions are taken.
- [ ] **The operator contact routes are listed**, so "tell every affected
      operator" is a list somebody works through.
- [ ] **The tabletop below has been walked**, with its date and outcome
      recorded.

**Recorded here:** Status: not done. Owner: [name]. Target date: [date].
Record each item above as it is completed. Each one is a human action against
an external body.

## The tabletop walk

Walk this runbook once before it is needed. The walk takes 45 minutes and
needs two maintainers and no infrastructure.

1. **The trigger.** Read this aloud, at a real wall-clock time: *"An hour ago
   a reporter mailed security@gradion.com saying they have seen a live
   exploitation of a Margince workspace-isolation bug against an installation
   that is not theirs. They have a working reproduction."*
2. **Fix T₀.** Both of you write down the awareness timestamp independently.
   They must match. If they do not, fix this runbook's definition of "aware"
   first.
3. **Case check.** Which of the three rows is this? (It is the overlap, and
   the answer everyone gives first is row one.)
4. **Fill the 24-hour skeleton** with what the trigger gave you, which is not
   much. The exercise finds which fields cannot be filled from a first report.
   A missing field must not stop the filing.
5. **Name the recipient and the route** without looking anything up. If
   nobody in the room can, the pre-registration step above is not done,
   whatever its checkbox says.
6. **Write the operator notice.** One paragraph, naming no installation.
7. **Record the outcome** in the pre-registration block above: the date, who
   walked it, and every field that could not be filled.

Steps 4 and 5 are what the walk tests. The rest sets them up.

## What this package does not do

- **No operator-facing page.** What a German operator reads about their own
  obligations belongs in the German jurisdiction pack, so the pack decides
  once how it presents operator obligations.
- **No legal advice.** This is an operational reading of Article 14 by the
  maintainers of the product. Where it and the regulation disagree, the
  regulation wins and this page is the thing to fix.
