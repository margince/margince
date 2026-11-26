---
jurisdiction: ie
version: 1
created: 2026-11-19
instruments:
  - name: "S.I. No. 336/2011, European Communities (Electronic Communications Networks and Services) (Privacy and Electronic Communications) Regulations 2011"
    provision: "Regulation 13"
    source-url: https://www.irishstatutebook.ie/eli/2011/si/336/made/en/print
marketing-exceptions: []
fixtures:
  - finding: objection_route
    triggers: ie/fixtures/advertising-without-objection-route.md
    clears: ie/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "S.I. No. 336/2011, European Communities (Electronic Communications Networks and Services) (Privacy and Electronic Communications) Regulations 2011"
    provision: "Regulation 13(1)"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "S.I. No. 336/2011, European Communities (Electronic Communications Networks and Services) (Privacy and Electronic Communications) Regulations 2011"
    provision: "Regulation 13(1)"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "S.I. No. 336/2011, European Communities (Electronic Communications Networks and Services) (Privacy and Electronic Communications) Regulations 2011"
    provision: "Regulation 13(1)"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "S.I. No. 336/2011, European Communities (Electronic Communications Networks and Services) (Privacy and Electronic Communications) Regulations 2011"
    provision: "Regulation 13(1)"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "S.I. No. 336/2011, European Communities (Electronic Communications Networks and Services) (Privacy and Electronic Communications) Regulations 2011"
    provision: "Regulation 13(12)"
---

# What Irish law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on Regulation 13 of the
European Communities (Electronic Communications Networks and Services) (Privacy and Electronic
Communications) Regulations 2011, S.I. No. 336 of 2011.

## Advertising to a company

Regulation 13 lets a sender advertise to an addressee who is not a natural person unless it has
objected. The engine cannot tell a company's general address from an individual's own address, so this
pack asks for consent for every address. That is stricter than the law, and the core already applies
it.

## The existing-customer exception

This layer states no exception. Regulation 13(11) adds a condition to the four of the EU layer: the
sale took place no more than 12 months before the message. The engine does not check when the sale
took place, so an exception here would send where Irish law refuses.

## Disclosures

The EU layer's disclosures apply. Regulation 13(12) also asks for a valid address to which the
recipient can send a request to stop.

## Where this layer takes the stricter reading

- The existing-customer exception is left out, because the 12-month condition cannot be checked.
- Advertising to a company address still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
