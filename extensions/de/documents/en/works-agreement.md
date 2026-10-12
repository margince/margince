# Works agreement on mail capture

> Reading copy. The [German version](../de/betriebsvereinbarung-vorlage.md) is
> the one to negotiate and sign. Sections 4 and 7 are the substance.

## Why it is co-determined

The system is objectively capable of monitoring conduct and performance, which
makes it co-determined under §87(1) Nr. 6 BetrVG, whether or not anyone
intends to monitor anybody.

## The two central clauses

**Section 4, purpose limitation and a prohibition on use.** Captured data may be
used only to document and work on business matters. Using it to assess an
individual's conduct or performance is not permitted, and the clause names what
that covers:

- the classifier's decisions about senders and threads,
- last-contact timestamps and any metric derived from them,
- relationship strength, warmth and coverage scores,
- the capture trace,
- the audit log.

The list names metadata because that is where performance monitoring would
happen. A clause that banned only "content" would leave it available.

**Section 7, artificial intelligence.** The classifiers that decide the
confidentiality of senders and threads run on models inside the company's own
infrastructure. Which tasks can send text to an external provider is set out in
`docs/reference/ai-egress.md`, generated from the configuration. Any change
that would let either of those two tasks send text outside requires the works
council's prior agreement.

## The rest

Scope, what is captured, visibility (default `classified`; `shared` only with
the works council's agreement), employee rights, retention, the council's
inspection rights, and term.

An administrator cannot read held content. The access check enforces this, not
the agreement.
