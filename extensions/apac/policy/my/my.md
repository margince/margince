---
jurisdiction: my
version: 1
created: 2026-11-19
instruments:
  - name: Personal Data Protection Act 2010 (Act 709)
    effective: 2013-11-15
    provision: "ss. 6 and 43"
  - name: Communications and Multimedia Act 1998 (Act 588)
    effective: 1999-04-01
    provision: "s. 233"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: objection_route
    marketing-only: true
fixtures:
  - finding: objection_route
    triggers: my/fixtures/advertising-without-objection-route.md
    clears: my/fixtures/advertising-complete.md
---

# What Malaysian law requires of an outbound message

Malaysia has no statute on advertising email. This layer rests on the Personal Data Protection Act
2010 (PDPA), and names s. 233 of the Communications and Multimedia Act 1998.

## Advertising needs consent

PDPA s. 6 asks for the data subject's consent to process their data, with exceptions for a
contract and a few other needs. Marketing is not among them, so the core's refusal of advertising
without a recorded consent stands. The layer states `marketing-exceptions` empty.

## Disclosures

PDPA s. 43 lets a data subject require, by written notice, that their data is not processed for
direct marketing. Advertising carries an `objection_route` so the notice can be given.

## What this layer does not state

- Communications and Multimedia Act s. 233 makes a communication that is obscene, false, menacing
  or offensive and meant to annoy an offence. It holds no rule on commercial email that the engine
  can apply.
- The PDPA reaches data processed in Malaysia. It does not follow a Malaysian recipient abroad, so
  `data-protection-follows-recipient` is off.

## Where this layer takes the stricter reading

- Every advertising mail carries an objection route, though s. 43 asks for written notice and not
  for a route in each mail.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
