---
jurisdiction: gr
version: 1
created: 2026-11-19
source-checked: 2026-11-25
instruments:
  - name: "Νόμος 3471/2006"
    provision: "άρθρο 11"
    source-url: https://www.dpa.gr/sites/default/files/2019-09/%CE%9D3471_06.PDF
    text-digest: 1685eb936dd5aa249acd2fa25c3d41f57fdbb3325ab36826514236b50fc3571d
    text-fetched: 2026-11-25
fixtures:
  - finding: objection_route
    triggers: gr/fixtures/advertising-without-objection-route.md
    clears: gr/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Νόμος 3471/2006"
    provision: "άρθρο 11 παρ. 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Νόμος 3471/2006"
    provision: "άρθρο 11 παρ. 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Νόμος 3471/2006"
    provision: "άρθρο 11 παρ. 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Νόμος 3471/2006"
    provision: "άρθρο 11 παρ. 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(4)"
---

# What Greek law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on article 11 of Law 3471/2006
on the protection of personal data in electronic communications.

## Advertising to a company

Article 11 asks for consent from every recipient, a natural or a legal person. That is what the EU
layer already applies.

## The existing-customer exception

Article 11(3) keeps the four conditions of the EU layer. This layer inherits the exception and adds
no key for it.

## Disclosures

The EU layer's disclosures apply.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
