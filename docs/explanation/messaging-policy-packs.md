<!-- describes: backend/internal/modules/consent backend/internal/shared/ports/messagingrules backend/internal/platform/messagingpolicy backend/internal/compose/extensionpolicy.go backend/internal/compose/requirementcheck.go backend/internal/modules/consent/countryquestion.go backend/internal/modules/activities/previewsend.go backend/internal/modules/activities/footerassessment.go backend/internal/modules/activities/draftcontent.go -->
# Where a pack's rules are read

A messaging policy pack is data: one country's `messaging.Rules`, compiled from the Markdown in an
extension's `policy/` folder. The pack decides nothing. The consent engine reads it at each step a
message passes, and every step asks the same door. [Update a messaging policy
pack](../how-to/update-a-messaging-policy-pack.md) says how a pack is written and changed;
[privacy-and-consent.md](privacy-and-consent.md) explains the engine the rules feed.

## From the extension to the registry

Each unit's `New()` returns its rule sets in `Messaging`, beside one jurisdiction pack per code. At
boot, `preflightMessagingRules` in `backend/internal/compose/extensionpolicy.go` validates each set,
refuses a code the unit carries no jurisdiction pack for (`requireOwnPack`), and refuses a code two
units declare. `backend/internal/compose/extensions.go` then registers each set with
`messagingrules.Register`.

The engine reads the registry through one function, `applicableRules` in
`backend/internal/modules/consent/ruleselectors.go`. It takes the codes a selector chose and
answers `messagingrules.Strictest` of them: the shortest window, every disclosure, the tightest cap,
an exception only where every pack grants it, and every distinct subject label in
`Applied.SubjectPrefixes`. A code with no pack contributes nothing, so the core's own baseline
applies to it.

## Which country judges a recipient

Two countries judge each recipient, each for its own kind of rule:

| Rule | Follows | Why |
|---|---|---|
| Marketing: consent or an exception, the advertising cap, the subject label, the objection route | The recipient's country | Anti-spam law applies where the mail is received. The sender's own anti-spam law does not follow its mail abroad. |
| The lawful basis: legitimate interest or not, the evidence windows, the lawful-basis warning | The recipient's country and the sending entity's, the stricter winning | The recipient's data-protection law reaches them where they are, such as GDPR Art. 3(2). The sender's binds the sender wherever the recipient is, such as GDPR Art. 3(1) or Vietnam's Law 91/2025/QH15 Art. 1. |
| Who the controller is, who answers about the data, request deadlines | The sending entity's country, except the deadlines below | The controller owes them under its own law. |

`recipientRequestCountry` in `backend/internal/modules/consent/pinnedsubject.go` resolves both
countries once per recipient. It takes the sending entity's country from `senderCountryOf` and puts
it in `RecipientCountry.Sender`. It picks the recipient's country from:

- a country the sender stated for this message, through `recipient_countries` on the send or the
  preview. `CountryOverridesFor` in `backend/internal/shared/ports/commsauthz/countryoverride.go`
  keeps each recipient's own statement, and `refuseAMachineStatedCountry` in
  `backend/internal/modules/consent/statedcountryauthor.go` admits it only from a human at the
  keyboard, never an agent;
- otherwise `recipientCountryTx` in `ruleselectors.go`, which reads the record: the contact's own
  address, then the address of the company they work for, or for a lead, the lead's company.

A record that states no usable code resolves to no marketing pack, and marketing to that recipient
needs their consent. The lawful basis is then judged under the sending entity's pack alone.

`RecipientCountry.dataProtectionCodes` in `ruleselectors.go` names the recipient's pack and the
sender's, and `applicableRules` folds them with `Strictest`. `settlePackBasisTx` in `backend/internal/modules/consent/recipientpack.go`
reads it: when either pack sets `no-legitimate-interests`, a legitimate-interest basis becomes the
recipient's consent, or no basis. So a sender in Vietnam writing to a recipient in Germany needs
consent or a case Vietnamese law names, although GDPR alone would accept a legitimate interest.
`packRulesFor` in `authorizewindows.go` takes the shorter evidence windows of the two packs. It takes
the marketing exception from the recipient's pack alone. `ConfirmSendWithoutLawfulBasis` in
`lawfulbasiswarning.go` asks the same fold before it records a confirmation.

A pack that sets `data-protection-follows-recipient` also takes the disclosures, the privacy page,
the mail language and the request deadlines for its recipients, whatever country the sender is in.
`recipientpack.go` holds that rule too.

## Opt-in and opt-out advertising

`marketing-mode` says what a recipient who has said nothing is owed. A pack that states none is
opt-in: advertising needs their consent or an exception the pack grants. A pack that states
`opt_out` allows advertising until they refuse.

`packRulesFor` in `backend/internal/modules/consent/authorizewindows.go` reads the mode from the
recipient's pack alone, and `marketingVerdict` in `backend/internal/modules/consent/verdict.go`
and `decideLeadOnItsRecord` in `backend/internal/modules/consent/authorizelead.go` apply it. An
objection, an unsubscribe, a withdrawal and a suppression block before the mode is read, so they win
in either mode. Mail that is not advertising never reads it. An opt-out allow records no lawful
basis, because none of the recorded values names "allowed until refused". Its decision row carries
the reason code `opt_out_allowed`. A lead takes its country from the same selector a contact does. `Strictest` keeps opt-out only where every pack that binds a send states it.

A recipient with no usable country gets no pack and so needs consent. Where an installed pack is
opt-out, that refusal could become an allow. The preview then sets `ask_country` on the recipient
(`countryCouldChangeTheAnswer` in `backend/internal/modules/consent/countryquestion.go`), and the
composer asks the sender for the country, only when the reader may update that contact. The answer is saved on the contact through the ordinary
contact edit, so it is audited like any other change, and the preview is asked again. No question
shows when the record states a country, when the sender cleared or stated one for the message, when
the refusal is not for want of consent, or when no installed pack is opt-out.

## The send preview

The composer asks before the sender presses Send. `Preview` in
`backend/internal/modules/activities/previewsend.go` builds the request from the send's own
builder, and `Gate.Preview` in `backend/internal/modules/consent/authorizepreview.go` decides each
recipient as staging will, inside a transaction it rolls back. So the preview and the refusal that
follows cannot disagree about an unchanged record. The preview also runs `withSubjectLabels`, the
subject-label check staging runs.

## Staging, when the sender presses Send

`stageDecisions` in `backend/internal/modules/consent/authorizestaging.go` decides each recipient
under their country and records one decision row each. The pack's rules decide:

- whether advertising without consent has a route: the existing-customer exception and each
  condition it names, read in `backend/internal/modules/consent/verdict.go` and
  `existingcustomerroute.go`;
- the refusal it gets without one. `requireConsentWhereNothingStandsIn` in
  `backend/internal/modules/consent/authorizeconsentonly.go` refuses with
  `marketing_consent_required` where `consent-only` is set. Where the pack offers an exception, or
  no pack applies, it refuses with `marketing_permission_required`;
- the subject label. `withSubjectLabels` in `backend/internal/modules/consent/requirements.go`
  refuses an advertising recipient whose label the subject does not start with
  (`subject_label_missing`), and every labelled recipient when the recipients owe different labels
  (`subject_labels_conflict`). No exception passes either, so the composer keeps the draft and the
  sender edits the subject or splits the recipients.

A refusal answers the Send with a 409 and a review the composer shows.

## Transmit, before the provider is called

`decideRecipients` in `backend/internal/modules/consent/authorizetransmit.go` asks the engine
again, because the record can change while a message waits. Two rules are read only here:

- the frequency cap. `applyFrequencyCap` and `lockCapAddresses` in
  `backend/internal/modules/consent/authorizecap.go` count the advertising each address received in
  the pack's window;
- the label on the finished message. `CheckMessage` in `requirements.go`, called through
  `backend/internal/compose/requirementcheck.go`, reads each advertising recipient's recorded codes
  and parks a delivery whose subject lacks a label. Staging refuses that message first, so this is
  the last check for a message that got past it.

`subjectLabelCheck` in `requirements.go` is the one label check that preview, staging and transmit
ask.

## The footer and its findings

A pack's `disclosures` say what a first message must carry. `owedDisclosures` in
`backend/internal/modules/consent/disclosurerender.go` takes the data-protection kinds
(`controller_identity`, `privacy_contact`) from the sender's pack, or from a recipient's pack that
follows the recipient, and every other kind from each recipient's pack. `layoutDisclosures` in
`disclosurelayout.go` checks each against the company's particulars, its `means` and the pack's
language. A missing one is a footer finding. `WithFooterFindings` in
`backend/internal/shared/ports/commsauthz/footer.go` turns an allow into a review with
`footer_incomplete`, which a sender may send past with a recorded reason. The preview fills them in
`backend/internal/modules/activities/footerassessment.go`.

## The draft content check

While a sender writes advertising, `CheckAdvertisingDraft` in
`backend/internal/modules/consent/draftcontent.go` runs `packDraftFindings` once per recipient pack.
It reads `claims-need-proof`, the pack's language and `foreign-advertiser-needs-local-agency`, and
answers notes above the send button. None of them stops the send. The activities module reaches it
through `backend/internal/modules/activities/draftcontent.go`.

## The citations in the warning

A pack may cite the provision behind a finding or a refusal, whether or not a review stamps it.
`citationsFor` and `provisionsShown` in `backend/internal/modules/consent/citation.go` look the code
up in the recipient's pack, for the sender's territory. For a finding about the lawful basis, a
restriction, a withdrawn consent or the controller's particulars, they also look it up in the sending
entity's pack (`senderCitesFinding`). The warning then names both laws, the recipient's first:
"(GDPR Art. 6(1); Law 91/2025/QH15 Art. 9)". A marketing finding cites the recipient's pack alone.

The answer carries every law in `provisions`, and the first one again in `provision` for older
clients. A citation is withheld for an instrument not yet in force or already expired, or for a
reviewed pack whose `review-due` has passed. The pack still applies in each of those cases; only the
citation is withheld.

The composer's warning also names the country the recipient was judged under and where it was read,
such as "Country: Germany, from the employer’s address" (`refusalCountryText` in
`frontend/src/features/mail/refusalcountry.ts`). The sender can correct it on the recipient's chip.

## Mail the engine sends for the pack

Two more rules change mail the product sends on its own:

- `opt-out-acknowledgement` queues a receipt with each refusal. `acknowledgeWithdrawalTx` in
  `backend/internal/modules/consent/optoutack.go` writes it, in the pack's time zone.
- `consent-confirmation-needs-request` holds a consent request until the contact's own request is on
  record, for the classes in `consent-request-first-for`. `mayAskByMail` in
  `backend/internal/modules/consent/confirmrules.go` decides it.

## What a decision records

Each decision row records the rules that judged its recipient. `rulesetStamp` in
`backend/internal/modules/consent/rulesetstamp.go` writes `ruleset_codes`, and for a single pack
its `ruleset_version` and `ruleset_digest`, the SHA-256 of the policy sources the set was compiled
from. So a question about one decision is answered by the code, the version and the digest, and
the digest tells which policy text produced the rules.

## The United States pack

`extensions/us/policy/us/us.md` states the CAN-SPAM Act (15 U.S.C. 7701 et seq.) and the Federal
Trade Commission's rule under it (16 CFR Part 316). It is opt-out, grants no exception and sets no
cap. It identifies advertising by a footer line, the `advertisement` disclosure ("This is an advertisement from" and the sender's legal name), the stricter reading of 7704(a)(5)(A)(i). It asks for no subject label. A commercial message owes a way to decline and the sender's postal address. The
postal address is the legal name and address of `controller_identity`, which the pack owes on
advertising only. `owedDisclosures` in `backend/internal/modules/consent/disclosurerender.go` reads
that disclosure from a recipient's pack when it is advertising-only, so a sender from a country
whose own pack asks for no address still owes it to a US recipient. The pack states no
data-protection key, because the United States has no federal law like the GDPR. A sender's own
country keeps its data-protection rules for the lawful basis.
