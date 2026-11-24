---
jurisdiction: be
version: 1
created: 2026-11-19
instruments:
  - name: "Code de droit économique / Wetboek van economisch recht"
    provision: "art. XII.13"
    source-url: https://www.ejustice.just.fgov.be/eli/loi/2013/02/28/2013A11134/justel
  - name: "Arrêté royal du 4 avril 2003 visant à réglementer l'envoi de publicités par courrier électronique"
fixtures:
  - finding: objection_route
    triggers: be/fixtures/advertising-without-objection-route.md
    clears: be/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Code de droit économique / Wetboek van economisch recht"
    provision: "art. XII.13, § 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Code de droit économique / Wetboek van economisch recht"
    provision: "art. XII.13, § 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Code de droit économique / Wetboek van economisch recht"
    provision: "art. XII.13, § 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Code de droit économique / Wetboek van economisch recht"
    provision: "art. XII.13, § 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Code de droit économique / Wetboek van economisch recht"
    provision: "art. XII.13, § 2"
---

# What Belgian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. XII.13 of the Code de
droit économique (Wetboek van economisch recht). The royal decree of 4 April 2003 on advertising by
electronic mail sets the exceptions.

## Advertising to a company

The royal decree lets a sender write to a legal person at an impersonal address, such as a general
company mailbox, unless it objects. The engine cannot tell a company's general address from an
individual's own address, so this pack asks for consent for every address. That is stricter than the
law, and the core already applies it.

## The existing-customer exception

Art. 1 of the royal decree keeps the four conditions of the EU layer. This layer inherits the
exception and adds no key for it.

## Disclosures

The EU layer's disclosures apply. Art. XII.13 and the royal decree also ask that every advertising
message states the right to object and gives an electronic means to do it.

## Where this layer takes the stricter reading

- Advertising to an impersonal company address still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
