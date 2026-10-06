<!-- prose:plain max-words=1000 -->
# Security Policy

Margince holds customer data. Its security model has one company per installation, and the server
refuses to start a second one in the same installation. Agents work under rules the server checks. We
welcome reports about holes in that model, and we take them seriously.

## Reporting a vulnerability

Report a vulnerability in private, through GitHub Security Advisories:
[open a draft advisory](https://github.com/margince/margince/security/advisories/new). You can also open
the "Security" tab of the repository and choose "Report a vulnerability". If you cannot use GitHub, mail
[security@gradion.com](mailto:security@gradion.com) instead. Do not open a public issue or pull request
for a security problem. A public report before the fix ships puts every installation at risk.

What to send:

- the endpoint, tool or part of the code that has the problem;
- the smallest way to show it, such as requests, payloads or a failing test;
- the harm it can do, such as a read of other workspaces, more rights than a user has, or an agent past
  its rules.

What you can expect:

- We answer within **3 business days** to say we have your report.
- Within **10 business days** we say whether it is in scope, and how serious we think it is.
- After that, we keep you up to date in the advisory until the fix ships.
- We name you in the advisory and the changelog, unless you ask us not to.

Margince is an early proof of concept, kept by a small team. So we do not promise a date for a fix. We
do promise to tell you where your report stands.

### If someone is using it right now

These times work for a normal report, but they are too slow when an attacker already uses the hole. So
there are two routes, and you choose which one you are on.

Put `ACTIVELY EXPLOITED` in the title of the advisory, or in the subject line if you mail us. Do this
only when you see the hole in use against a real installation. To think that someone could use it is not
enough. We answer that report within **4 business hours**. We answer with what we need from you, not
with a ruling on how serious it is.

Two more things then happen. First, our own 24-hour clock to report it starts as soon as we hold a
report we trust, before we confirm it. So we may ask you questions before we know much. Second, the
advisory may be public before the normal time. Once attackers use a hole, the teams that run Margince
need to know so they can fix it, and silence helps only the attacker. We tell you before that happens.

What we do, and the dates we must meet: [docs/compliance/cra/README.md](docs/compliance/cra/README.md).

## Scope

In scope is a problem that breaks a security rule this code says it keeps, such as:

- **Workspace isolation**: a read or write of data of other workspaces. Each installation holds one
  workspace, so the installation is the line that keeps one workspace from the others. No table uses
  row-level security or has a workspace field. Every query must go through the one workspace transaction
  helper, which refuses a call with no workspace. So a path that leaves that helper is itself the hole.
- **Row scope and RBAC**: access to records outside the own, team or all scope of the caller. This
  includes error, replay and conflict paths. An answer outside the scope must be 404, so no caller learns
  that the record is there.
- **Agent rules**: an agent that runs a 🟡 action with no approval, uses one approval twice or after the
  content changed, or approves its own work. Also an agent with more rights than the user who gave them,
  or a change the server accepts with no declared tier.
- **Sign-in**: a forged session or passport, session fixation, or a revoked credential that still works.
- **The write shape**: a change that skips the audit row or the outbox row, or that takes its source from
  the request body.
- **Injection and SSRF** in a handler, tool or connector.

Out of scope:

- a hole in a third-party package, unless you show harm here (report it to that project);
- a finding that needs the attacker to own the host or the database already;
- denial of service against a dev installation (`MARGINCE_ENV=dev` turns off some trust checks by
  design).

## Supported versions

Margince is an early proof of concept: **only the `main` branch is supported**. There are no release
branches yet, and we do not copy fixes to older versions. Fixes go to `main`.

## No bounty

There is no bug bounty program today and no promise of money. We name you in the advisory and the
changelog, and we name each reporter whose report we fix, unless they ask us not to.

## Safe harbour

We will not take or support legal action against you if you report in good faith under this policy. Good
faith here means:

- you test only against your own installation;
- you do not access data that is not yours;
- you do not run a denial of service against an installation you do not run;
- you do not make the problem public before a fix ships, or before we agree on a date.

Report soon once you find something, and give us time to fix it.
