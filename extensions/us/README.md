<!-- prose:plain -->
# The `us` pack

What the law of the United States requires of an outbound message, stated as data that the core
engines apply. The pack decides nothing: it declares, and the engine reads.

The pack sits in `policy/us/`. Its layer, `us/us.md`, names the CAN-SPAM Act of 2003
(15 U.S.C. 7701 et seq.) and the Federal Trade Commission's rule under it (16 CFR Part 316).

| Rule | What the pack states |
|---|---|
| Advertising | `marketing-mode: opt_out`: allowed until the recipient refuses, with no exception to name |
| A way to decline | `objection_route`, on advertising |
| The sender's postal address | `controller_identity` (legal name and address), on advertising |
| Data protection | No key: the United States has no federal law like the GDPR |
| Retention | One tax-record floor of 7 years from the end of the year, in `retention.go` |

A refusal always stops advertising, in this pack as in every other. The pack is a draft: it states
no `reviewed` or `reviewer`, and the release refresh checks its sources.

How a pack is changed: [update-a-messaging-policy-pack.md](../../docs/how-to/update-a-messaging-policy-pack.md).
