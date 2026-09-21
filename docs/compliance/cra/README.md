# Reporting an actively exploited vulnerability (CRA Article 14)

**This is our duty as the manufacturer, not the customer's paperwork.** The
four documents in [`../en/`](../en/README.md) are what an installation signs
before it reads employee mail. This directory is what **we** do when a
weakness in Margince is being exploited in the wild.

The obligation is Article 14 of Regulation (EU) 2024/2847 (the Cyber
Resilience Act). Two dates, and they do different jobs:

- **11 September 2026** — when Article 14 starts applying. It is in force now.
- **11 December 2027** — the transitional cutoff. Article 69(3) extends the
  Article 14 reporting duty to in-scope products **placed on the market before**
  that date, so a product older than the regulation's substantive requirements
  still carries the duty to report.

The regulation governs; this page is the operational reading of it, written so
nobody has to do that reading with a 24-hour clock already running.

## The three cases, and which one you are in

Decide this first, because two of the three carry no clock for us.

| What happened | Whose duty | Our clock |
| --- | --- | --- |
| A vulnerability in Margince is being **actively exploited** | ours, as manufacturer | **yes** — start it now |
| A customer's installation was breached through something that is not a Margince weakness (a stolen credential, their own infrastructure) | theirs, as operator | none |
| A Margince vulnerability is being exploited **at a live installation** | **both, together** | **yes**, and see the warning below |

In the overlap case the two duties fire at once and they point in opposite
directions. Ours is to file. Theirs is to patch and, where their own law says
so, to report. **Notify the operators that they must patch; do not name any
installation, customer or affected party in the filing.** The authority is
told about the product and the exploitation, not about who was hit.

## When the clock starts

The clock starts when we **become aware**, and "aware" is earlier than it sounds: it is the moment a maintainer holds a credible report that a weakness
in Margince is being exploited, not the moment it is confirmed, reproduced,
triaged or understood.

A report that arrives at 23:00 on a Friday starts the clock at 23:00 on that
Friday. There is no business-hours clause in Article 14 and this runbook does
not invent one.

Write the awareness timestamp down before anything else — it is the field
every one of the three reports is measured against, and reconstructing it
afterwards from a mail header is exactly the argument nobody wants to have.

## The three reports

Each one is a separate filing with its own deadline, measured from awareness
(T₀). All three are filled skeletons in this directory — open the file, fill
the bracketed slots, file it.

| # | Report | Deadline | Template |
| --- | --- | --- | --- |
| 1 | Early warning | **T₀ + 24 hours** | [early-warning-24h.md](early-warning-24h.md) |
| 2 | Vulnerability notification | **T₀ + 72 hours** | [vulnerability-report-72h.md](vulnerability-report-72h.md) |
| 3 | Final report | **14 days** after a corrective or mitigating measure is available | [final-report.md](final-report.md) |

The first two are due whether or not the vulnerability is understood by then.
An early warning that says "we know it is being exploited and little else" is
the report Article 14 asks for at 24 hours; a late one that says everything is
still a late one.

A **severe incident affecting the security of the product** follows the same
24-hour and 72-hour rhythm, and its final report is due **one month** after
the 72-hour notification rather than 14 days. Everything else on this page
applies unchanged.

## Where it goes

Two recipients, **simultaneously**: the CSIRT designated as coordinator in the
member state of our main establishment, and ENISA. Article 16 establishes a
single reporting platform through which both are reached, so in practice this
is one submission with two addressees.

**Which coordinator is a rule, not a fact about us.** Article 14(7) names the
CSIRT of the member state where the manufacturer's **product-cybersecurity
decisions are predominantly taken**. Where that member state cannot be
determined, the chain continues: the EU establishment with the most employees,
then — for a manufacturer with no EU main establishment — the authorised
representative, the importer, the distributor, and last the member states where
users are.

On the first limb ours is Germany, and the basis is worth writing down rather
than assuming: the decisions about this product's security are taken where it
is built. So the coordinator is **BSI** (Bundesamt für Sicherheit in der
Informationstechnik), whose operational arm is CERT-Bund. Its published
reporting channel is on `bsi.bund.de`; the exact submission address is recorded
in the pre-registration step below rather than transcribed here, because an
address copied into a document and never checked is worse than a document that
says where to look.

**Re-decide it when the basis moves, not when somebody remembers.** A second
establishment, a move of where security decisions are taken, or an EU presence
arriving or leaving changes which CSIRT is the right recipient — and a
notification to the wrong one is a notification not made. The pre-registration
block below records the basis beside the address, so the two are re-checked
together.

## And tell the operators

Separately from the filing, and **without undue delay** once we are aware:
tell whoever runs a Margince installation that there is an actively exploited
vulnerability, what mitigates it, and what corrective measure is coming or
available. Article 14(8) puts that duty on the manufacturer, and filing does
not discharge it — a report to an authority protects nobody's installation.
Where we fail to do it in time, the notified CSIRT may do it for us.

**The owner is whoever fixed T₀.** The maintainer who started the clock owns
the operator notice until it is sent, because the failure this prevents is a
pair of maintainers each assuming the other told the customers.

**The channel, and its known reach.** The first channel is the GitHub Security
Advisory the private report already lives in
([SECURITY.md](../../../SECURITY.md)), published at the point where publishing
helps operators more than it helps an attacker who is already exploiting it. In
an actively-exploited case that point arrives early: the exploit is in use, so
the secrecy is protecting nothing. Publishing also puts the vulnerability where
automation can read it, which is the machine-readable form Article 14(8) asks
for where appropriate.

**An advisory reaches whoever watches this repository, which is not the same
set as whoever runs the software.** A self-hosted installation subscribes to
nothing by default. So the advisory is the floor rather than the answer: every
operator we hold a contact route for is told directly, by that route, and the
notice carries the same three things — the risk, the mitigation available now,
and the corrective measure.

Where no route exists for an installation, that is a gap in what we know about
our own deployments rather than a discharge of the duty, and the honest record
of it belongs in the final report's *users informed* block.

## Before any of this happens

Three things cannot be done under a running clock. They are done once and
checked when they change.

- [ ] **The reporting account exists.** Registered on the Article 16 single
      reporting platform, with the submission route and the credential holder
      recorded here, and at least one other maintainer able to reach it.
- [ ] **The coordinator's channel is confirmed** against BSI's own published
      contact, and the date of that check is written next to it. An authority's
      address is not a constant.
- [ ] **The Article 14(7) basis is recorded** — which limb of the rule selects
      our coordinator, and why — and re-checked whenever an establishment, or
      where product-security decisions are taken, changes.
- [ ] **The operator contact routes are listed**, so "tell every affected
      operator" is a list somebody works through rather than a hope about who
      reads an advisory.
- [ ] **The tabletop below has been walked**, with its date and outcome
      recorded.

**Recorded here:** *(not yet — this package was written before its
pre-registration step was executed. The three boxes above are the remaining
work, and each is a human action against an external body that no document can
perform for itself.)*

## The tabletop walk

An untested runbook is a document, not a capability. The walk is 45 minutes,
needs two maintainers and no infrastructure.

1. **The trigger.** Read this aloud, at a real wall-clock time: *"An hour ago
   a reporter mailed security@gradion.com saying they have seen a live
   exploitation of a Margince workspace-isolation bug against an installation
   that is not theirs. They have a working reproduction."*
2. **Fix T₀.** Both of you write down the awareness timestamp independently.
   They must match. If they do not, this runbook's definition of "aware" is
   not doing its job and that is the first thing to fix.
3. **Case check.** Which of the three rows is this? (It is the overlap, and
   the answer everyone gives first is row one.)
4. **Fill the 24-hour skeleton.** With what the trigger actually gave you —
   which is not much. The point of the exercise is to find out which fields
   cannot be filled from a first report, and whether that stops the filing. It
   must not.
5. **Name the recipient and the route.** Without looking anything up. If
   nobody in the room can, the pre-registration step above is not done,
   whatever its checkbox says.
6. **Write the operator notice.** One paragraph, naming no installation.
7. **Record the outcome** in the pre-registration block above: the date, who
   walked it, and every field that could not be filled.

What the walk is for is step 5 and step 4's gaps. Everything else is warm-up.

## What this package deliberately does not do

- **No operator-facing page.** What a German operator reads about their own
  obligations belongs in the German jurisdiction pack, so the pack decides
  once how it presents operator obligations rather than one page inventing its
  own shape.
- **No legal advice.** This is an operational reading of Article 14 by the
  maintainers of the product. Where it and the regulation disagree, the
  regulation wins and this page is the thing to fix.
