---
jurisdiction: tw
version: 1
created: 2026-11-19
instruments:
  - name: Personal Data Protection Act
    effective: 2012-10-01
    provision: "arts. 19 and 20"
    source-url: https://law.moj.gov.tw/LawClass/LawAll.aspx?pcode=I0050021
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: objection_route
    marketing-only: true
fixtures:
  - finding: objection_route
    triggers: tw/fixtures/advertising-without-objection-route.md
    clears: tw/fixtures/advertising-complete.md
citations:
  - code: marketing_objection
    territorial-test: any_sender
    instrument: "Personal Data Protection Act"
    provision: "art. 20(2)"
  - code: opted_out_after_request
    territorial-test: any_sender
    instrument: "Personal Data Protection Act"
    provision: "art. 20(2)"
  - code: objection_route
    territorial-test: any_sender
    instrument: "Personal Data Protection Act"
    provision: "art. 20(3)"
---

# What Taiwanese law requires of an outbound message

Taiwan has no statute on advertising email: a draft anti-spam act was never passed. This layer rests
on the Personal Data Protection Act (個人資料保護法) as amended, in force from 1 October 2012.

## Advertising needs consent

Art. 20 lets a non-government agency use personal data for marketing within the purpose it was
collected for, and otherwise with the data subject's consent. The engine cannot tell the purpose of
collection from a record. So the core's refusal of advertising without a recorded consent stands,
and the layer states `marketing-exceptions` empty.

## Disclosures

Art. 20(2) and (3): once the data subject refuses marketing, it stops. The first marketing message
offers a way to refuse at the sender's cost. Advertising carries an `objection_route`.

## What this layer does not state

- Art. 51 reaches a non-government agency abroad that handles the data of Taiwanese nationals. The
  pack does not set `data-protection-follows-recipient`, because that key would also move request
  deadlines and the privacy page to this pack, which states neither.

## Where this layer takes the stricter reading

- Marketing within the purpose of collection is allowed by art. 20 and is not offered here, because
  the engine cannot check that purpose.
- Every advertising mail carries an objection route, though art. 20(3) asks for one in the first
  marketing message.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
