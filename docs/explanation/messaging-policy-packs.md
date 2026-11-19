<!-- describes: backend/internal/modules/consent backend/internal/shared/ports/messagingrules backend/internal/platform/messagingpolicy backend/internal/compose/extensionpolicy.go backend/internal/compose/requirementcheck.go backend/internal/modules/activities/previewsend.go backend/internal/modules/activities/footerassessment.go backend/internal/modules/activities/draftcontent.go -->
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

Rules follow the recipient, not the sender. `recipientRequestCountry` in
`backend/internal/modules/consent/pinnedsubject.go` picks one country per recipient:

- a country the sender stated for this message, through `recipient_countries` on the send or the
  preview. `CountryOverridesFor` in `backend/internal/shared/ports/commsauthz/countryoverride.go`
  keeps each recipient's own statement, and `refuseAMachineStatedCountry` in
  `backend/internal/modules/consent/statedcountryauthor.go` admits it only from a human at the
  keyboard, never an agent;
- otherwise `recipientCountryTx` in `ruleselectors.go`, which reads the record: the contact's own
  address, then the address of the company they work for, or for a lead, the lead's company.

A record that states no usable code resolves to no pack, and marketing to that recipient needs
their consent.

A pack that sets `data-protection-follows-recipient` also takes the disclosures, the privacy page,
the mail language and the request deadlines for its recipients, whatever country the sender is in.
`backend/internal/modules/consent/recipientpack.go` holds that rule. A pack that sets
`no-legitimate-interests` has `settlePackBasisTx` in the same file turn a legitimate-interest basis
into the recipient's consent, or into no basis.

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

A reviewed pack may cite the provision behind a finding or a refusal. `citationsFor` and
`citationFor` in `backend/internal/modules/consent/citation.go` look the code up in the recipient's
pack, for the sender's territory. They show none for a draft pack, a pack whose `review-due` has
passed, or an instrument not yet in force or already expired. The pack still applies in each of
those cases; only the citation is withheld.

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
