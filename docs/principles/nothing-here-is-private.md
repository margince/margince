# Nothing here is private

**This repository is public. Everything in it is readable by anyone, forever,
including what you delete tomorrow.**

The binding form is *This repository is public* in the rulebook. What follows is
the reasoning and the method.

## Two obligations, one reader

The reader this principle protects is **a public contributor with no access to
anything but this tree**. Both obligations follow from them existing:

- **Never refer to a private repository**, document, path or link: not in code,
  comments, tests, docs, issues, commit messages or PR bodies. If a rule
  matters, write the rule out here. Citing somewhere they cannot reach is the
  same as not stating the rule, and it also tells them they are not the
  intended audience.
- **Never include local machine paths or secrets.**

A decision number (`ADR-0054`) may appear as a label, but never cite it as
though a reader could open it: the records are not in this tree. Write the rule
itself out, here.

## The method

**Assume the automated net is partial**, because it is.
`TestPublicTreeCitesNothingPrivate` catches a private repository name, a
`specs/` path or a `foundation#NNNN` reference, but only in the file types it
scans: `.go .md .yml .yaml .json .ts .tsx .sh .sql .css`. A reference in HTML or
a `.mjs` script walks straight past it. It also does **not** read
commit messages or PR bodies, and it has no pattern for a secret or a machine
path. Those stay your judgement, with the secret-scan gate
as the only other net.

**A vulnerability is not reported here.** An exploitable weakness goes to a
private GitHub Security Advisory, never a public issue or pull request, because a
public report before a fix ships puts every deployment at risk. The test is the
one [SECURITY.md](../../SECURITY.md) implies: if you can write the reproduction,
it belongs in an advisory. A cross-tenant read, a row-scope or
RBAC escape, an agent-governance bypass, a forged or still-binding revoked
credential, a mutation that skips the audit or outbox row, injection, SSRF.

The `security` label is for hardening and defence-in-depth carrying no live
exploit. Using it to file a working exploit publishes the exploit.

**Write the reason down** when the reason is the rule. The reasoning behind most
guardrails is not in this repository. A gate usually tells you what to do
instead. Where a rule is judgement and has no gate
([derive the obligation](derive-the-obligation.md#what-this-does-not-ask-for)),
write it out here in full. If you cannot tell why a rule exists and it changes
your patch, ask in the issue.

## What this does not ask for

- **Not secrecy about the product.** The design, the contract and the reasoning
  in these docs are meant to be read. This is about not *pointing* at things a
  reader cannot open, and not leaking material that harms anyone.
- **Not avoiding security work in the open.** Hardening lands here in public,
  with its test. Only live, reproducible weaknesses take the private path.
