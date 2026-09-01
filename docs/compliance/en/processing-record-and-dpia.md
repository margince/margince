# Processing record and impact assessment

> Reading copy. The [German version](../de/verarbeitungsverzeichnis-und-dsfa.md)
> is the one to file. **Every row there names the code path that enforces it**,
> and each was checked to exist — the difference between a record describing the
> product and one describing an intention is whether a regulator can open the
> file.

## Four processing operations

1. **Capturing business correspondence** — the CRM record of what was said, on
   the employment basis plus the statutory retention duty.
2. **Holding captured messages** — data minimisation. Visibility is derived as
   the strictest thing any capturing mailbox asks for, and checked on every
   read.
3. **Automatic classification of senders and threads** — deciding whether a
   sender is a business contact and a thread ordinary business. Models run
   locally. The Senders page shows every decision and a person's correction is
   final.
4. **Destruction on the employee's own request** — text, provider original,
   attachments and their files, vectors, delivery copies. Commercial
   correspondence inside its statutory window is **not** destroyed and is
   reported as skipped.

## The one property worth reading twice

**Unavailable or out of budget means held, never released.** A classifier that
cannot run, a model nobody bound, an answer below the confidence floor, an
unparseable reply — every one of them leaves the message private. Exactly one
answer opens a thread; every other kind holds it, and there is no fallback
branch that could turn an unrecognised answer into an opening one.

## Residual risk, stated rather than argued away

**A new sender is held, not absent.** Their first message is stored until a
decision is reached.

**The classifier is a model and will be wrong on some threads.** The asymmetric
floor biases errors towards holding; it does not remove them.

**Metadata remains expressive.** Who corresponded with whom and when is a
statement even without content. Only the works agreement's prohibition on use
addresses that, and it is a contractual control rather than a technical one.

**The product verifies none of this.** It will connect a mailbox whether or not
this record exists.
