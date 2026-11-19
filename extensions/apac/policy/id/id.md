---
jurisdiction: id
version: 1
created: 2026-11-19
instruments:
  - name: "Law No. 11 of 2008 on Electronic Information and Transactions, as amended by Law No. 19 of 2016 and Law No. 1 of 2024"
    provision: "art. 26"
  - name: Law No. 27 of 2022 on Personal Data Protection
    effective: 2022-10-17
    provision: "arts. 9 and 20"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: objection_route
    marketing-only: true
fixtures:
  - finding: objection_route
    triggers: id/fixtures/advertising-without-objection-route.md
    clears: id/fixtures/advertising-complete.md
---

# What Indonesian law requires of an outbound message

Indonesia has no statute on advertising email. This layer rests on art. 26 of the Law on Electronic
Information and Transactions (UU ITE) and on the Personal Data Protection Law (UU PDP, Law No. 27 of
2022). UU PDP took effect on 17 October 2022, and its two years of transition ended on 17 October
2024.

## Advertising needs consent

UU ITE art. 26 asks for the person's consent to use their personal data by electronic means. UU PDP
art. 20 lists other grounds too, but the layer reads art. 26 as binding marketing by email. The core
refuses advertising without a recorded consent, and the layer states `marketing-exceptions` empty.

## Disclosures

UU PDP art. 9: the data subject can withdraw consent. Advertising carries an `objection_route`.

## What this layer does not state

- UU PDP art. 20 includes legitimate interests as a ground, so `no-legitimate-interests` is off.
- UU PDP art. 2 reaches a controller abroad whose processing affects Indonesia. The pack does not
  set `data-protection-follows-recipient`, because that key would also move request deadlines and
  the privacy page to this pack, which states neither.

## Where this layer takes the stricter reading

- Marketing by email needs consent, though UU PDP alone might allow it on legitimate interests.
- The UU ITE as amended states no effective date here, because the amendments came in stages.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
