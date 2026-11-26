---
jurisdiction: lu
version: 1
created: 2026-11-19
source-checked: 2026-11-25
instruments:
  - name: "Loi du 30 mai 2005 concernant la protection de la vie privée dans le secteur des communications électroniques"
    provision: "art. 11"
    source-url: https://legilux.public.lu/eli/etat/leg/loi/2005/05/30/n4/jo
    text-url: https://data.legilux.public.lu/file/eli-etat-leg-loi-2005-05-30-n4-jo-fr-html.html
    text-digest: f89fd8a5ae616d83f90cb2e562a543f0fbce70dd08c5cd609d55d74dc91aa5bb
    text-fetched: 2026-11-25
fixtures:
  - finding: objection_route
    triggers: lu/fixtures/advertising-without-objection-route.md
    clears: lu/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Loi du 30 mai 2005 concernant la protection de la vie privée dans le secteur des communications électroniques"
    provision: "art. 11, paragraphe 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Loi du 30 mai 2005 concernant la protection de la vie privée dans le secteur des communications électroniques"
    provision: "art. 11, paragraphe 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Loi du 30 mai 2005 concernant la protection de la vie privée dans le secteur des communications électroniques"
    provision: "art. 11, paragraphe 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Loi du 30 mai 2005 concernant la protection de la vie privée dans le secteur des communications électroniques"
    provision: "art. 11, paragraphe 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Loi du 30 mai 2005 concernant la protection de la vie privée dans le secteur des communications électroniques"
    provision: "art. 11, paragraphe 4"
---

# What Luxembourg law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 11 of the law of 30
May 2005 on privacy in electronic communications.

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

Art. 11(2) keeps the four conditions of the EU layer. This layer inherits the exception and adds no
key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- Whether art. 11 binds advertising to legal persons was not checked; consent is asked for every
  address.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
