# Processing record and impact assessment

> Reading copy. The [German version](../de/verarbeitungsverzeichnis-und-dsfa.md)
> is the one to file. Each row there names the source file that enforces it,
> under `backend/internal/`. `backend/gates/processingrecord_test.go` fails
> when a named file no longer exists.

## Four processing operations

1. **Capturing business correspondence:** the CRM record of what was said, on
   the employment basis plus the statutory retention duty.
2. **Holding captured messages:** data minimisation. Visibility is derived as
   the strictest thing any capturing mailbox asks for, and checked on every
   read.
3. **Automatic classification of senders and threads:** deciding whether a
   sender is a business contact and a thread ordinary business. Models run
   locally. The Senders page shows every decision and an employee's correction is
   final.
4. **Destruction on the employee's own request:** text, provider original,
   attachments and their files, vectors, delivery copies. Commercial
   correspondence inside its statutory window is **not** destroyed and is
   reported as skipped.

## Failure holds the message

Unavailable or out of budget means held, never released. A classifier that
cannot run, a model nobody bound, an answer below the confidence floor and an
unparseable reply all leave the message private. One answer opens a thread; every other kind holds it, and there is no fallback
branch that could turn an unrecognised answer into an opening one.

## Residual risk

**A new sender is held.** Their first message is stored until a
decision is reached.

**The classifier will be wrong on some threads.** The asymmetric
floor biases errors towards holding; it does not remove them.

**Metadata remains expressive.** Who corresponded with whom and when is a
statement even without content. Only the works agreement's prohibition on use
addresses that, and it is a contractual control.

**The product verifies none of this.** It will connect a mailbox whether or not
this record exists.
