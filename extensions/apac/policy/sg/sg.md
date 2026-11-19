---
jurisdiction: sg
version: 1
created: 2026-11-19
instruments:
  - name: Spam Control Act 2007
    effective: 2007-06-15
    provision: "s. 11 and Second Schedule"
    source-url: https://sso.agc.gov.sg/Act/SCA2007
  - name: Personal Data Protection Act 2012
    effective: 2014-07-02
    provision: "ss. 13 to 15"
    source-url: https://sso.agc.gov.sg/Act/PDPA2012
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: objection_route
    marketing-only: true
subject-prefix: "<ADV>"
fixtures:
  - finding: objection_route
    triggers: sg/fixtures/advertising-without-objection-route.md
    clears: sg/fixtures/advertising-complete.md
  - finding: subject_prefix
    triggers: sg/fixtures/advertising-without-label.md
    clears: sg/fixtures/advertising-complete.md
---

# What Singapore law requires of an outbound message

This layer rests on the Spam Control Act 2007 and the Personal Data Protection Act 2012 (PDPA).

## Advertising needs consent

The Spam Control Act lets unsolicited commercial email go out in bulk if it meets the Second
Schedule. The PDPA still needs consent to use a person's email address for marketing (s. 13). Deemed
consent (s. 15) covers a purpose that is obvious when the person gave the address. Marketing is
seldom that purpose, so this layer offers no exception. The core refuses advertising without a
recorded consent.

## The subject label (Second Schedule)

Unsolicited commercial email carries `<ADV>` and a space at the start of its subject. A subject
without it is held.

## Disclosures (Second Schedule)

The email carries an unsubscribe facility and a statement, in English, that the recipient may use
it (`objection_route`). An unsubscribe request is honoured within 10 business days. The core stops
advertising when the request is recorded, which meets that.

## What the engine cannot hold

- The engine checks that the subject starts with `<ADV>`. It does not check the space after it.
- The PDPA Do Not Call registry covers Singapore phone numbers and not email.

## Where this layer takes the stricter reading

- The Act binds unsolicited commercial email sent in bulk. The label and the unsubscribe facility
  are asked of every advertising mail here, because the engine cannot count a sender's bulk.
- The PDPA gives no route that a sale opens, so the layer states `marketing-exceptions` empty.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
