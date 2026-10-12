<!-- prose:plain -->
# Nothing here is private

**This repository is public. Every reader can read all of it, for all time, including what you
remove the next day.**

The rule a change must follow is *This repository is public* in the rulebook. What follows is the
reason and the way to check it.

## Two obligations, one reader

The reader this principle is for is **an outside contributor who sees only this tree**. Both obligations follow from that reader:

- **Never point to a private repository**, document, path or link: not in code, comments, tests,
  docs, issues, commit messages or PR bodies. If a rule counts, write the rule out here. To point
  to a place they cannot reach is the same as not stating the rule. It also tells them the page is not
  for them.
- **Never include local machine paths or secrets.**

A decision number (`ADR-0054`) may show up as a label. Never point to it as if a reader could open
it: the records are not in this tree. Write the rule itself out, here.

## How to check it

**Expect the gates to miss things**, because they do. `TestPublicTreeCitesNothingPrivate`
finds a private repository name, a `specs/` path or a `foundation#NNNN` reference. It only does so
in the file types it scans: `.go .md .yml .yaml .json .ts .tsx .sh .sql .css`. A reference in HTML or
a `.mjs` file goes right past it. It also does **not** read commit messages or PR bodies, and it
has no pattern for a secret or a machine path. These are your job, and the secret-scan gate is the
only other check.

**A vulnerability is not reported here.** A hole an attacker can use goes to a private GitHub
Security Advisory, never to a public issue or pull request. A public report before a fix ships puts
every installation at risk. The test follows from [SECURITY.md](../../SECURITY.md): if
you can write the steps to show it, it goes in an advisory. Some examples: a read of data in some other tenant, a
way out of row scope or RBAC, or a way past agent rules. Also a forged credential, a revoked one
that still works, a mutation that skips the audit or outbox row, injection, or SSRF.

The `security` label is for work that makes the code safer, such as a second check behind a first
one. It holds no working attack. To file a working attack under that label makes the attack public.

**Write the reason down** when the reason is the rule. The reasons behind most guardrails are not in
this repository. A gate most often tells you what to do instead. Where a rule needs a human to decide and
has no gate ([derive the obligation](derive-the-obligation.md#what-this-does-not-ask-for)), write all
of it out here. If you cannot tell why a rule exists and it changes your work, ask in the issue.

## What this does not ask for

- **Not keeping the product secret.** The design, the contract and the reasons in these docs are
  there to read. This is about not *pointing* at things a reader cannot open, and not putting
  out content that harms someone.
- **Not hiding security work.** Work that makes the code safer is merged here in public, with its
  test. Only holes that are open now, and that someone can show with steps, take the private path.
