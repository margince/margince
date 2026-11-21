# Update a messaging policy pack

A messaging policy pack states one country's rules for outbound mail as data: whether advertising
needs consent, which exception may stand in for it, what a message must disclose, and the subject
label. The core engine applies the data. [Where a pack's rules are read](../explanation/messaging-policy-packs.md)
says where.

Follow these steps to change a rule, add a country, or roll a change back.

## Where the packs live

Each unit holds its packs in `policy/`, one folder per country, named for its lower-case ISO 3166-1
code:

- `extensions/eu/policy/<code>/`: the EU member states. Each one lists the shared layer
  `extensions/eu/policy/regional/eu.md` first.
- `extensions/apac/policy/<code>/`: the Asia-Pacific countries. Each one lists its own layer only.

A country folder holds `index.md`, its country layer `<code>.md`, and its own `fixtures/`. The top of
a unit's `policy/` holds what its packs share: `fallback.md`, `regional/`, `fixtures/` and
`research/`. The compiler reads no file in `research/`, so notes go there.

A unit with an `index.md` at the top of `policy/` holds one pack instead. `CompileUnit` in
`backend/internal/platform/messagingpolicy/unit.go` reads both layouts.

## Layers, and why a later one may only tighten

`index.md` lists the layers of one pack in its front matter, most general first:

1. `fallback.md`, a floor every pack of the unit shares;
2. `regional/<name>.md`, the law of a region, such as `regional/eu.md`;
3. `<code>/<code>.md`, the country layer, which the pack has one of;
4. `<code>/overlay/<name>.md`, a narrower rule on top of the country.

The compiler applies them in that order. Each key a layer states replaces the one below, except
`instruments` and `citations`, which add to it. After each layer it asks `Loosened` in
`backend/internal/shared/ports/messagingrules/tighten.go`, and refuses a layer that loosens the one
below it. A country layer can drop the existing-customer exception that `regional/eu.md` grants. It
cannot add an exception the region refuses, or remove a disclosure.

In a unit with shared layers, every pack lists every shared layer, in one order. The compiler
refuses a pack that leaves one out, and a file in `policy/` that no index lists.

## The keys

The keys a layer may state are the YAML tags of `layerFile` in
[`backend/internal/platform/messagingpolicy/source.go`](../../backend/internal/platform/messagingpolicy/source.go).
Each one becomes the field of `messaging.Rules` with the same name in
[`backend/pkg/extension/messaging/messaging.go`](../../backend/pkg/extension/messaging/messaging.go),
whose comment says what the engine does with it. An unknown key fails the compile.

A country layer must state `jurisdiction`, `version` and `created`, and `jurisdiction` must match
its file name. Every layer names at least one fixture pair, and its Markdown body explains the
layer to its reviewer.

The Korean layer, `extensions/apac/policy/kr/kr.md`, shows the common keys:

```yaml
---
jurisdiction: kr
version: 1
created: 2026-11-19
instruments:
  - name: "Act on Promotion of Information and Communications Network Utilization and Information Protection"
    effective: 2014-11-29
    provision: "art. 50"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
consent-only: true
disclosures:
  - kind: objection_route
    marketing-only: true
  - kind: advertiser_contact
    marketing-only: true
subject-prefix: "(광고)"
opt-out-acknowledgement: true
opt-out-acknowledgement-within: 336h
opt-out-acknowledgement-utc-offset: 9h
fixtures:
  - finding: subject_prefix
    triggers: kr/fixtures/advertising-without-label.md
    clears: kr/fixtures/advertising-complete.md
---
```

- `instruments` name the laws the rules rest on, with the date each took effect. They are for the
  reader of a decision; the engine applies the keys below them.
- `marketing-exceptions` lists the routes to advertising without consent. `[]` states that there is
  none. An `existing_customer` entry names each condition it needs, such as
  `requires-sale-evidence`.
- `consent-only` adds that no one-message exception a sender records stands in for consent.
- `disclosures` are what a first message carries. `marketing-only` limits one to advertising, and
  `means` asks the text to name a web page, an email address or a phone number.
- `subject-prefix` is the label an advertising subject starts with.
- The `opt-out-acknowledgement` keys owe a receipt for a refusal, the hours before an unsent one
  raises an alarm, and the time zone it prints.

## Fixtures

A fixture is a draft message in Markdown. Its front matter holds `subject`, `advertising`, `sender`,
`mail-language` and the `particulars` the footer prints: `legal-name`, `postal-address`,
`privacy-contact`, `objection-route` and `advertiser-contact`. The body is the draft text.

Each layer names fixture pairs. A pair has a `finding`, a draft that `triggers` it and a draft that
`clears` it, and both sit under the unit's `fixtures/` or the country's own. The finding is a
disclosure kind, `subject_prefix`, or a draft finding declared in
`backend/internal/modules/consent/draftcontent.go`.

`TestEveryPolicyFixturePairSplitsOnItsFinding` in
`backend/internal/modules/consent/policyfixtures_test.go` compiles every pack and runs each pair
through the engine's own draft, subject and footer checks. It fails when the trigger draft does not
raise the finding, or the clearing draft does. The compiler refuses a layer with no pair, a pair
that names one draft twice, and a draft that cannot be read.

Add a pair for each finding your layer adds. Write the draft in the country's language when the
finding depends on wording, as the `(광고)` pair does.

## Change a rule

1. Edit the layer. Raise its `version` by one when the rules change, because each decision records
   the version it was judged under (`ruleset_version`). Text in the body alone needs no new version.
2. Add or change the fixture pair for the finding you touched.
3. Say in the body which act and article the change rests on, and where you took the stricter
   reading.
4. Regenerate the unit's `messagingrules_gen.go`: run `go generate` in `extensions/<unit>`, or
   `make -C backend gen` for every unit.
5. Update the unit's own test where it pins the rule: `extensions/eu/eu_test.go`,
   `extensions/eu/de_test.go`, `extensions/apac/apac_test.go` or `extensions/apac/vn_test.go`.
6. Run the checks below.

These checks catch a mistake:

| Check | Fails when |
|---|---|
| `make -C backend messaging-rules-drift` (`gen-messaging-rules -check`) | a policy does not compile, or `messagingrules_gen.go` differs from it |
| `make -C backend drift` | any generated file differs; it runs the check above first |
| `GOWORK=off go test ./...` in `extensions/<unit>` | the unit's pinned rules, its country list, or a rule set the boot would refuse |
| `go test ./internal/modules/consent -run PolicyFixture` in `backend/` | a fixture pair does not split on its finding |
| `go test ./internal/modules/consent -run Shipped` in `backend/` | a pack gives another outcome than `shippedOutcomes` records |

`shippedOutcomes` in `backend/internal/modules/consent/packoutcomes_test.go` writes each pack's
outcome as an answer: its windows, its exception, the refusal advertising without consent gets, its
cap and its label. A change that moves an outcome must change that entry too.

## Add a country

1. Create `extensions/<unit>/policy/<code>/index.md`. Copy a neighbour's and list the unit's shared
   layers, then `<code>/<code>.md`.
2. Write `<code>/<code>.md` with `jurisdiction`, `version: 1`, `created`, the `instruments`, the rule
   keys and its fixture pairs. Name the act in the body.
3. Write the fixture drafts under `<code>/fixtures/`.
4. Run `go generate` in `extensions/<unit>`. The unit's `New()` builds one jurisdiction pack per
   compiled rule set, so the policy folder is the one list of countries.
5. Add the code to the country list of the unit's test: `memberStates` in
   `extensions/eu/eu_test.go`, or `countries` in `extensions/apac/apac_test.go`. Add it to the
   tests there that list which packs offer an exception or a label.
6. Add the country to `shippedOutcomes` in `backend/internal/modules/consent/packoutcomes_test.go`.
   `shippedPackCodes` gives each shipped pack a test code from `shippedPackCodeRanges` in the same
   file. When the packs outgrow those ranges the test says so; add a range outside the ones
   `testCodeRanges` hands out, and `backend/gates/messagingtestcodes_test.go` checks that the two
   tables do not overlap.
7. Run the checks above.

One unit owns everything for one code. `requireOwnPack` in
`backend/internal/compose/extensionpolicy.go` refuses messaging rules for a code the unit carries no
jurisdiction pack for, and the boot refuses two units that declare one code.

The installation can then choose the code in `workspace.country`, at the setup claim or on the
Jurisdiction settings page; see [Deploy Margince](deploy-margince.md#choose-the-country).

## Citations

`citations` link an engine code to the provision a rep is shown beside the warning. Each entry
names:

- `code`: the refusal or finding it explains, such as `no_marketing_consent` or `objection_route`.
  `TestEveryCitationExplainsACodeTheEngineEmits` in `backend/gates/messagingruleapplied_test.go`
  refuses a code the engine never reports.
- `territorial-test`: `any_sender`, `sender_here` or `sender_elsewhere`, for where the sending legal
  entity is established.
- `instrument`: the `name` of one instrument the compiled pack declares, in this layer or one below.
- `provision` and `source-url`: the part of the law, and its official text over https. Either may be
  left out to take the instrument's own.

```yaml
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Gesetz gegen den unlauteren Wettbewerb (UWG)"
    provision: "§ 7 Abs. 2 Nr. 2"
```

One code is cited once per sender across all layers. A citation in `regional/eu.md` therefore holds
for every member state, and a country layer cites only the codes the region leaves out.
`Validate` in `backend/pkg/extension/messaging/provenance.go` checks each citation when the pack
compiles.

## Drafts and reviewed packs

A pack cites its law whether or not a review stamps it. The packs are refreshed against their
sources at each release, and no human sign-off is a condition for a citation. These keys record a
review where one took place, and the compiler takes the weakest value across the layers:

- `reviewed`: the date a named reviewer approved the layer. A pack with no date is a draft.
- `reviewer`: who approved it. A review with no reviewer is refused.
- `source-checked`: the date the instruments were last compared with their published texts. The
  refresh writes it. A reviewed pack that cites must have checked its sources on or before its
  review.
- `review-due`: when the next review of a reviewed pack is owed. Once it passes, the pack still
  applies and its citations are withheld. A draft has no review to fall due.

`citationFor` in `backend/internal/modules/consent/citation.go` decides when a citation is shown. It
withholds one whose instrument is not yet in force or has expired, and every citation of a reviewed
pack whose `review-due` has passed.

## Sources and the refresh

`make policy-refresh` compares every pack with its official sources. It is a step of each release;
[Cut a release](cut-a-release.md#refresh-the-policy-packs) says when to run it and how to read its
report. The command reads and writes these keys of an instrument:

- `source-url`: the official page of the law, which a rep is linked to.
- `text-url`: where the refresh reads the text instead, over https. State it when the official page
  loads its text with a script, and a PDF or an XML form of the same law exists.
- `text-digest`: the SHA-256 of the text the refresh last read, in lower-case hex.
- `text-fetched`: the day of that read.
- `checked-by-hand`: the day someone read a source that no plain HTTP request can read. The refresh
  does not fetch such an instrument, and it states no `text-url` and no `text-digest`.

```yaml
instruments:
  - name: "Gesetz gegen den unlauteren Wettbewerb (UWG)"
    provision: "§ 7"
    source-url: https://www.gesetze-im-internet.de/uwg_2004/__7.html
    text-digest: 5f2b8a0c9d3e4f6a7b8c9d0e1f2a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c
    text-fetched: 2026-11-21
```

You write `text-url` and `checked-by-hand`. The refresh writes `text-digest`, `text-fetched` and the
`source-checked` of the layer, so do not type those. The digest above is an example and not the
digest of that page.

How the refresh reads a source:

- It sends one GET request and runs no script. `httpSource` in
  [`backend/tools/policy-refresh/main.go`](../../backend/tools/policy-refresh/main.go) is that
  request.
- From a page it drops scripts, styles and tags, and joins the white space. It then takes the
  digest of that text. So two reads of one page give one digest while the law stays the same.
- A page with fewer than 500 characters of text holds no law, and the report says `not read`.
- Another kind of answer, such as a PDF, is digested as its bytes.

`RefreshSources` in
[`backend/internal/platform/messagingpolicy/refresh.go`](../../backend/internal/platform/messagingpolicy/refresh.go)
does this. It sets `source-checked` on a layer when it read every instrument of the layer, or the
instrument is checked by hand. The date is the day of the run, or the oldest `checked-by-hand` of
the layer when that is earlier. A layer with an instrument it could not read keeps its old date.

The refresh changes those lines and leaves every other line of the file as it is. It needs the
instruments as a block list, with one `- name:` item for each instrument, as above.

A refresh changes the text of a layer and not its rules, so it needs no new `version`. The digest
of the pack does change, which is why the command builds `messagingrules_gen.go` again. A refresh of
a reviewed pack moves `source-checked` past `reviewed`. The compiler refuses that until a reviewer
sets `reviewed` again.

The compiler accepts an instrument with none of these keys, and the shipped packs had none when the
keys were added. `Validate` in `backend/pkg/extension/messaging/provenance.go` checks the keys an
instrument does state: a digest has its day, and one instrument states a digest or a check by hand,
never both.

## Roll a change back

In this tree, revert the commit that changed the policy, run `go generate` in the unit, and commit
the regenerated file with it.

An installation built from its own repository requires each unit in its `instance/go.mod`. To
return to an earlier pack, require the earlier version of that unit, such as
`github.com/margince/margince/extensions/apac`, and build again; see
[Install extensions as Go modules](install-extensions-as-go-modules.md). A decision keeps the
version and the digest of the rules it was judged under, so decisions taken before the rollback
still name the rules that judged them.
