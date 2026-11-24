<!-- prose:plain -->
# The `us` pack

What the law of the United States requires of an outbound message, stated as data that the core
engines apply. The pack decides nothing: it declares, and the engine reads.

The pack sits in `policy/us/`. Its layer, `us/us.md`, names `CAN-SPAM Act of 2003` (`15 U.S.C. 7701`
and the sections after it) and `16 CFR Part 316`, the rule under it.

| Rule | What the pack states |
|---|---|
| Advertising | `marketing-mode: opt_out`: allowed until the recipient refuses, with no exception to name |
| Label | `subject-prefix: "ADV:"` (`15 U.S.C. 7704(a)(5)(A)(i)`) |
| A way to refuse | `objection_route`, on advertising |
| The postal address | `controller_identity` (legal name and address), on advertising |
| Privacy law | No key: no law of the whole country works like the GDPR |
| Retention | One floor for accounting records of 8 years from the end of the record's year, so 7 years after the end of the following year, in `retention.go` |

A refusal always stops advertising, in this pack as in every other. The pack is a draft: it states
no `reviewed` or `reviewer`, and the release refresh checks its sources.

How a pack is changed: [update-a-messaging-policy-pack.md](../../docs/how-to/update-a-messaging-policy-pack.md).
