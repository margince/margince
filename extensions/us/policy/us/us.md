---
jurisdiction: us
version: 2
created: 2026-11-24
source-checked: 2026-11-25
instruments:
  - name: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    effective: 2004-01-01
    provision: "15 U.S.C. 7704(a)(3), (a)(4) and (a)(5)"
    source-url: https://uscode.house.gov/view.xhtml?req=granuleid:USC-prelim-title15-section7704&num=0&edition=prelim
    text-url: "https://www.govinfo.gov/link/uscode/15/7704?type=usc&year=mostrecent&link-type=html"
    text-digest: 6c95bd853551c56281aee1f2285e61a1572ce2dd388df7ba1303088ae6c37f3f
    text-fetched: 2026-11-25
  - name: "CAN-SPAM Rule (16 CFR Part 316)"
    effective: 2008-07-07
    provision: "16 CFR 316.5"
    source-url: https://www.ecfr.gov/current/title-16/chapter-I/subchapter-C/part-316
    text-url: "https://www.ecfr.gov/api/renderer/v1/content/enhanced/current/title-16?part=316"
    text-digest: 191be6209091480fd834d97a1accba754589d43003e5537ec2f2c6c57c2acf12
    text-fetched: 2026-11-25
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-mode: opt_out
marketing-exceptions: []
disclosures:
  - kind: advertisement
    marketing-only: true
  - kind: controller_identity
    marketing-only: true
  - kind: objection_route
    marketing-only: true
fixtures:
  - finding: controller_identity
    triggers: us/fixtures/advertising-without-postal-address.md
    clears: us/fixtures/advertising-complete.md
  - finding: advertisement
    triggers: us/fixtures/advertising-without-legal-name.md
    clears: us/fixtures/advertising-complete.md
  - finding: objection_route
    triggers: us/fixtures/advertising-without-objection-route.md
    clears: us/fixtures/advertising-complete.md
citations:
  - code: objection_route
    territorial-test: any_sender
    instrument: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    provision: "15 U.S.C. 7704(a)(5)(A)(ii)"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    provision: "15 U.S.C. 7704(a)(3)"
  - code: controller_identity
    territorial-test: any_sender
    instrument: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    provision: "15 U.S.C. 7704(a)(5)(A)(iii)"
  - code: advertisement
    territorial-test: any_sender
    instrument: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    provision: "15 U.S.C. 7704(a)(5)(A)(i)"
  - code: marketing_objection
    territorial-test: any_sender
    instrument: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    provision: "15 U.S.C. 7704(a)(4)(A)"
  - code: opted_out_after_request
    territorial-test: any_sender
    instrument: "CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.)"
    provision: "15 U.S.C. 7704(a)(4)(A)"
---

# What United States law requires of an outbound message

This layer rests on the CAN-SPAM Act of 2003 (15 U.S.C. 7701 et seq.) and the Federal Trade
Commission's rule under it (16 CFR Part 316). It is the first pack whose market is opt-out.

## Advertising needs no prior consent

The Act does not ask for the recipient's consent before a commercial email. It lets the sender write
until the recipient refuses. `marketing-mode: opt_out` states that. The engine then allows
advertising to a recipient here whose record holds no refusal. No exception is needed, so
`marketing-exceptions` is stated and empty.

A refusal always wins. An unsubscribe, an objection, a withdrawal and a suppression stop advertising
here as they do everywhere, and no setting turns that off.

## What the message must carry

- 15 U.S.C. 7704(a)(5)(A)(i): clear and conspicuous identification of the message as an
  advertisement. The Act asks for it unless the recipient gave prior affirmative consent. The engine
  cannot tell the two cases apart, so the pack asks for it on every advertisement. The identification
  is a line in the footer, "This is an advertisement from" and the sender's legal name
  (`advertisement`). That is the stricter reading. The pack asks for no subject label.
- 15 U.S.C. 7704(a)(5)(A)(ii) and (a)(3): a working way to decline further email, free and with no
  barrier (`objection_route`, on advertising).
- 15 U.S.C. 7704(a)(5)(A)(iii): a valid physical postal address of the sender. The pack asks for
  `controller_identity`, which is the legal name and the postal address, on advertising only.

## What the engine holds without a key

- 7704(a)(4)(A) and 16 CFR 316.5: a refusal is honoured within 10 business days. The core stops
  advertising when the refusal is recorded, which meets it.
- 7704(a)(2): a subject line must not mislead. The engine cannot judge that, so the layer states no
  key for it.

## Data protection

The United States has no federal law like the GDPR. The layer states no data-protection key:
the recipient's pack does not take the privacy page or the request deadlines, and it does not forbid
legitimate interests. A sending entity in another country keeps its own pack's data-protection
rules for the lawful basis, because the stricter pack wins there.

## Where this layer takes the stricter reading

- The advertisement line is asked for on every advertisement, not only where there is no consent.
- Replies and deal follow-ups keep the core windows. The Act sets none.
- State laws on top of the federal Act, such as a state's own commercial-email rules, are not
  stated. The layer follows the federal Act alone.
- The retention floor, in `retention.go`, is 7 years after the end of the calendar year following
  the record's year, so it never releases before 7 years after the return's filing. That is the
  longest common federal period, for bad-debt and worthless-security claims. State law is not
  modelled.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so it is refreshed against its
sources at each release and a legal review adds the date.
