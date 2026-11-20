---
jurisdiction: hr
version: 1
created: 2026-11-19
instruments:
  - name: "Zakon o elektroničkim komunikacijama (NN 76/22)"
    source-url: https://narodne-novine.nn.hr/clanci/sluzbeni/2022_07_76_1116.html
fixtures:
  - finding: objection_route
    triggers: hr/fixtures/advertising-without-objection-route.md
    clears: hr/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Zakon o elektroničkim komunikacijama (NN 76/22)"
    provision: "članak 50. stavak 1."
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Zakon o elektroničkim komunikacijama (NN 76/22)"
    provision: "članak 50. stavak 1."
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Zakon o elektroničkim komunikacijama (NN 76/22)"
    provision: "članak 50. stavak 1."
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Zakon o elektroničkim komunikacijama (NN 76/22)"
    provision: "članak 50. stavak 1."
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Zakon o elektroničkim komunikacijama (NN 76/22)"
    provision: "članak 50. stavak 3."
---

# What Croatian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on the Zakon o elektroničkim
komunikacijama (Electronic Communications Act, NN 76/22), which carries Art. 13 into Croatian law.
The review must name the article.

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- The article number in the 2022 act was not checked; the review must add it.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
