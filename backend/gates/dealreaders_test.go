// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// `deal` is an RBAC object, and until this gate nothing held the object half of
// that anywhere outside the module that owns it.
//
// A deal is the commercial position: which account is being sold to, for how
// much, at what stage and by when. The grant exists because a figure is the
// most quotable thing in a CRM — a seat with no business in the pipeline should
// not be able to read the number, the close date, or the name of what somebody
// else is working on.
//
// Two gates cover the compose tier and both say in their own words that they do
// not cover this: rbacgate_test.go judges only exported methods on a module's
// *Store or *Service, and composerowscope_test.go covers ROW scope and refuses
// to count object admission as satisfaction. Between them the object half had
// no fitness function outside internal/modules/deals.
//
// Third census over objectGate, after edgereaders_test.go's shape and the lead
// and company slices. `deal` is the widest of the four by reach rather than by
// count: it is read from twelve packages that do not own it, and the three
// ungated reads it turned up were all in that outer ring — an agent tool, an
// account page's advice band, and a pre-meeting brief.

import (
	"go/ast"
	"path"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// dealTable is the table this census is about. Both the pattern and the
// failure messages below read it, so neither can drift from the subject.
const dealTable = "deal"

// dealGate is the subject. No platform spelling gateSeeds it: like the lead and
// the company, a deal has no bespoke admission helper — auth.Require(ctx,
// "deal", …) and auth.ReadGranted(ctx, "deal") ARE the object gate, and
// objectGate reads both off the call.
//
// No row-half allowance either, and here that is the whole point: a row clause
// standing in for the object half is precisely what the three ungated reads
// below turned out to be, each one composing auth.ScopeClauseFor and stopping
// there.
var dealGate = objectGate{
	object:              dealTable,
	literal:             gatekit.TableReadPattern(dealTable),
	objectGateSatisfies: true,
}

// predicateDealReads: the deal table appears only inside a JOIN or EXISTS that
// selects or routes records the caller is separately gated on, and nothing about
// the deal reaches the response. The test that puts a read here rather than in
// the gated set: removing the deal condition could only WIDEN the result the
// caller already sees.
var predicateDealReads = gatekit.Waive(map[string]string{
	"internal/compose/company360/contacts.go:contactDealRoles":                "the roster's deal-role column: `r.contact_id, r.deal_id, r.role` off the relationship table, with the deal joined only for its row scope so a seat on a deal the caller may not open is absent. The projection is the EDGE, and the deal id it carries is one the row scope already admitted",
	"internal/compose/company360/roleproposalwrite.go:ownWords":               "the quoted sentence a role proposal rests on — who said it and what they said — with the deal joined to bound which conversations count. The projection is an activity's subject and body under the activity grants; no deal column is selected",
	"internal/compose/contact360/nextmeeting.go:nextMeetingSection":           "the contact's next meeting: `a.id, a.occurred_at, a.subject`, with the deal joined only to reach the meetings that hang off one. Activity columns under the activity grant, and removing the join would widen the meetings considered",
	"internal/compose/dealownerseam.go:dealOwner":                             "reads `owner_id` — a USER id, the colleague who owns the deal — so a seam can route work to them. No deal column is selected and what the caller learns is who to ask",
	"internal/compose/network/companycoverage.go:companyStakeholderEdge":      "the stakeholder-edge predicate inside the account coverage read: whether a contact holds a seat on a live deal at this account. The deal appears only inside the join condition, and what the coverage answers is about CONTACTS, under the grants that read owns",
	"internal/compose/reportprojects.go":                                      "the project report's deal sums, `sum(d.amount_minor_base)` over a project's open and won deals. Aggregates rather than rows, composed by the report the project grant admits, and the figures are the project's own commercial size",
	"internal/compose/signalscan.go:scanGhostedThreads":                       "the ghosted-thread scan asks which OPEN deals have gone quiet so a signal can be raised. It runs on the scan's schedule and what it writes is a signal row; the signal is then read through the signal grant, which SignalScopeClause resolves back to the subject's own visibility",
	"internal/compose/slippingnextstep.go:dealsWithNoOpenNextStep":            "selects `d.id` and nothing else — which deals carry no open next step, so the slipping-next-step pass can raise one. No deal column reaches a caller; what does is a task on the deal the pass already decided about",
	"internal/compose/stageevidenceread.go:criteriaOf":                        "reads a deal's current `stage_id` so the evidence criteria for THAT stage can be looked up. One id leaves it, to choose a row in the criteria catalog, and the criteria are workspace configuration rather than deal data",
	"internal/compose/stageprogression.go:refuseAStaleAutomaticMove":          "reads a deal's `pipeline_id` to refuse an automatic stage move whose pipeline has changed underneath it. Its only effect is a REFUSAL — the move does not happen — and the executor runs under the system principal",
	"internal/modules/commissions/entryfieldmask.go:dealAmountExcludedClause": "the arm that takes a ledger entry out when the deal's amount is withheld from this reader: `EXISTS (SELECT 1 FROM deal d WHERE d.id = deal_id AND ...)` over the write-authority predicate auth renders. No deal column is selected and none reaches the response — the entry's own columns do, under the commission grant the read already holds. Removing the condition could only WIDEN the ledger, which is the disclosure it exists to close",
	"internal/modules/activities/attachment.go:accountRollUp":                 "resolves `company_id` from the deal an attachment's activity is linked to, so the file rolls up to the right account. One id leaves it, inside the upload's own write",
	"internal/modules/activities/capturedfiles.go:accountForCapturedActivity": "the same `company_id` resolution for captured files, on the capture path. One id, consumed by the write that files them",
	"internal/modules/activities/companyscope.go":                             "the account-scope fragment: which activities belong to an account, reached through the link's deal among other ends. It is a clause that only ever narrows the activities a caller is shown, never a reader of deal columns",
	"internal/modules/activities/lasttouch.go:lastTouchCandidateQuery":        "the live-account CTE of the same cold-queue selector: a company carries an open deal, which is what makes it an account worth a reminder. The deal is reached only inside that CTE and no deal column is selected — what leaves the query is a company id the account arm then tests",
	"internal/modules/activities/lasttouch.go:lastTouchEligibility":           "the deal arm of the cold-queue selector, the twin of the company and lead arms the other censuses file the same way: the deal's status and age decide which QUEUE ENTRIES are stale enough to surface, and no deal column reaches the caller. The selector now also COLLAPSES a deal onto the account absorbing it, so removing the arm no longer merely widens the queue — it restores the duplicate entries the collapse exists to remove. Either way no deal column leaves the query, and the entry that survives sits on a company the caller already reaches",
	"internal/modules/activities/lasttouch.go:openChildReminderHoldsAccount":  "the same cold-queue selector's hold, reading deal.company_id to ask whether an account's queue entry is already represented by an unanswered one on a record it absorbs. The deal is reached only inside a NOT EXISTS and no deal column is selected; the arm's only effect is to WITHHOLD a queue entry, never to surface one",
	"internal/modules/activities/projectcoverage.go:ProjectActivityFactsTx":   "the project's activity facts, reaching deals only to find the conversations near a delivery. The projection is activity counts and timestamps; removing the deal arm would count more activities, never disclose a deal",
	"internal/modules/activities/responsemetrics.go":                          "the deal arm of \"is this a commercial thread\" in the first-response metrics. What reaches the caller is a count and a median over their own activity scope, never a deal column, and removing the arm would widen the set measured",
	"internal/modules/activities/waitingsql.go":                               "the deal arm of the waiting-thread worklist, off the gated link join: it decides which threads belong to live commercial business, and what reaches the caller is the thread their own activity scope already admits",
	"internal/modules/capture/sinkproject.go:dealProject":                     "resolves which PROJECT a deal-linked activity belongs to, so captured mail is filed against the right delivery. It selects `p.id` under the project row scope and the deal is a join hop; `LIMIT 2` so an ambiguous answer is refused rather than guessed",
	"internal/modules/consent/authorizeevidence.go:liveDealInLinks":           "an EXISTS asking whether the evidence a send rests on still names a live deal, so a stale authorization is refused. One boolean, and its only effect is to REFUSE a transmit — the reverse of a disclosure",
	"internal/modules/consent/authorizevalidators.go:validateQuote":           "the same shape for the quoted evidence: an EXISTS that refuses an authorization whose quote no longer resolves. A boolean that only ever narrows what may be sent",
	"internal/modules/contacts/demote.go:ensureNoLiveDeal":                    "the guard that refuses demoting a contact who still holds a live deal. An EXISTS whose only effect is a refusal, reached from DemoteLead past the lead object gate and its row probe",
	"internal/modules/contacts/projectcompany.go:RemoveProjectCompany":        "`count(*) FROM deal` — the guard that refuses unstaffing a company a project still has open deals with. A number that only ever KEEPS the edge, inside a write that asks the relationship and project grants",
	"internal/modules/contracts/visibility.go:VisibleClause":                  "the contract visibility predicate. A contract hangs off a deal or, failing that, an account, so the clause asks whether the caller may see one of the two and admits the CONTRACT on that basis. Every column it selects belongs to the contract; the deal is reached only by EXISTS",
	"internal/modules/dealrooms/room_list.go:roomPage":                        "the room list. `roomColumns` selects `r.*` only — the room's own columns, including the `deal_id` the room already carries — and `JOIN deal d` is there for dealScopeClause, so a room on a deal the caller may not open is absent. No deal column is projected",
	"internal/modules/dealrooms/room_read.go:readRoom":                        "one room, same projection and same reason: the deal join carries the room's row visibility and contributes no column of its own",
	"internal/modules/dealrooms/room_write.go:resolveSteward":                 "reads the deal's `owner_id` so a new room is stewarded by whoever owns the deal it is about. A USER id, inside CreateDealRoom's own write",
	"internal/modules/deals/stageremoval.go:refuseIfOccupied":                 "`count(*) FROM deal WHERE stage_id = $1` — the guard that refuses archiving a stage deals still sit in. Reached from ArchiveStage past auth.Require(pipeline, delete), and its only effect is a refusal",
	"internal/modules/deals/stages.go:refuseTerminalWithOpenDeals":            "the twin guard: a stage cannot be made terminal while open deals occupy it. Same shape, past auth.Require(pipeline, update), and the number never leaves as anything but a refusal",
	"internal/modules/privacy/erasure_selectors.go:notTransitivelyHeld":       "the legal-hold exclusion reads `deal.legal_hold` to REFUSE erasing an activity still held through a deal. Its only effect is to keep a row: no deal column leaves the predicate and the statement it joins is an erase bound",
	"internal/modules/privacy/retention_floor.go":                             "the retention floor fragment: how long a class of record must be kept, reaching the deal to find the commercial correspondence a statutory floor applies to. It only ever KEEPS rows",
	"internal/modules/privacy/retentionrestricted.go:notHeldThroughAnyLink":   "the same legal-hold exclusion for the retention selectors. A deal it could not see would be a row deleted under hold",
	"internal/modules/projects/delivery.go:lockedDealProject":                 "reads a won deal's `project_id` under its lock so delivery starts against the right project, or mints one. One id, inside StartDeliveryForWonDeal's own write",
	"internal/modules/search/graphcompanyreach.go":                            "the company-reach fragment, walking activity links through their deal end to find who at an account has been spoken to. The projection is contacts and counts under the grants search owns",
	"internal/modules/signals/warmroom.go:RouteInEdges":                       "`SELECT d.id FROM deal d WHERE d.company_id = $1` inside an EXISTS — whether the account has live business, which decides which CONTACTS are offered as a route in. It asks auth.Require(contact, read) itself and the projection is contacts; no deal column is selected",
	"internal/modules/webhooks/deliveryvisibility.go:dealWorkedBy":            "the delivery visibility probe: an EXISTS under auth.OwnerScopeClauseFor that answers ErrNotFound for a deal outside the subscriber's own work. A refusal, and the one that keeps a webhook from carrying a deal its owner may not read",
	"internal/platform/auth/signalscope.go:SignalScopeClause":                 "the signal row-scope clause itself, which resolves a signal about a deal to the deal's own visibility. It is a clause rather than a read: it only ever narrows the signals a caller is shown",
})

// lifecycleDealReads: sweeps, scans, stamps, lawful-processing exports and
// seeders. Each is gated by the WRITE it belongs to, or runs as PrincipalSystem
// â which auth.Require short-circuits outright (platform/auth/rbac.go), so the
// gate would admit them anyway and the entry records why asking was never the
// point.
var lifecycleDealReads = gatekit.Waive(map[string]string{
	"internal/compose/assuranceseam.go":                                        "the assurance scan's subject statement, selecting a deal's owner, amount and currency so a finding can be raised against it. assurancebundle.go records that the pass runs as PrincipalSystem over the whole installation; what a human later reads is AssuranceExceptions, which asks both halves",
	"internal/compose/captureofflinedemo.go:fillParties":                       "the offline capture demo's fixture parties, read to seed a mailbox that has something to capture against. Demo seeding rather than a product read path, where the demo binds its own principal",
	"internal/modules/activities/retentionstamp.go:StampCorrespondenceForDeal": "the retention stamp freezes the deal's NAME into the evidence at the moment the correspondence qualifies, because a rename or a delete must not take the proof with it. It runs inside the qualifying write and the name goes onto the evidence row, never to a caller",
	"internal/modules/automation/automations_preview.go:dealPreviewDefs":       "the catalog entries whose blast-radius preview ranges over the deal table. resolvePreviewRecipe asks auth.Require(ctx, def.table, ActionRead) before running any of them, and def.table is \"deal\" for exactly these — the object arrives as a struct FIELD, which is why no static reader can see the gate from the definition",
	"internal/modules/deals/closedatesweep.go:stageVelocityDays":               "the stage-velocity measure the close-date sweep rests on — how long deals sit in each stage, via `lead()` over the stage history. It runs on the sweep's schedule and produces a number per stage rather than a deal",
	"internal/modules/deals/reconcile.go:reconcileCandidatesSQL":               "the follow-up reconciler's candidate scan: deals whose evidence suggests a follow-up was missed. It runs per workspace on the reconciler's schedule and what it writes is a task",
	"internal/modules/privacy/erasure_restrict.go:stampLegacyHandelsbriefe":    "the restriction pass stamps legacy commercial letters with the floor that holds them, reaching the deal to decide which correspondence that is. It runs as the system principal on the installation sweep and its effect is a stamp that KEEPS rows",
	"internal/modules/privacy/retentionselectors.go":                           "the retention selectors, which select the IDS a retention rule is due to act on under the system principal. No deal column reaches any caller — the output is a delete or scrub bound — and a selector respecting a caller's grants would under-delete",
	"internal/modules/privacy/sarsections.go:sarRecordSections":                "the deal rows AS the export: what Art. 15 owes a data subject about the deals they are named on, so a grant that narrowed it would make the answer wrong rather than safer. Runs as the system principal on a request a human already authorised",
	"internal/modules/search/embedgen.go":                                      "the per-entity source text the embedding lane indexes, mirroring the deal's search_tsv columns so the vector and lexical lanes agree about one text. It runs on the indexing path under the system principal; the SEARCH that later reads the index row carries the caller's own scope",
})

// calleeGatedDealReads: a helper whose every caller asks the deal object gate at
// the entry point above it.
//
// What each entry has to establish, and what the reason therefore names: EVERY
// caller of this function asks the deal object gate. One that does not makes the
// entry false, and the entry is where a reviewer checks it â this census
// resolves gates FORWARD and cannot resolve them caller-ward, because the call
// graph is by NAME and a gated Store.X would then vouch for an ungated
// Handlers.X.
var calleeGatedDealReads = gatekit.Waive(map[string]string{
	"internal/compose/briefs/briefcontinuity.go:previousRanking":           "the previous run's ranking, so a brief can say what moved. The briefs package asks the deal object gate at its own entry points",
	"internal/compose/briefs/brieflineage.go:briefLineage":                 "a brief item's lineage across runs, behind those same gated entry points",
	"internal/compose/briefs/briefreads.go:briefCandidates":                "the candidate deals a brief ranks, behind the gated brief entry",
	"internal/compose/briefs/briefreads.go:briefRevenueNorm":               "the revenue normalisation the ranking divides by, behind the same gate",
	"internal/compose/briefs/briefstore.go:readRunItems":                   "the items of one brief run, read back for display or for the mail. Its callers ask the deal object gate; the mail path runs under the mailer's own principal",
	"internal/compose/company360/pipelineread.go:openPipeline":             "the open-pipeline rows the advice, state-strip and dismissal paths rest on. Its ONE caller is gatherSuggestionInputs, which reads auth.ReadGranted(ctx, \"deal\") into in.pipeline and calls this only when it holds — so a seat with no deal grant is advised about everything else and told nothing about the pipeline. The census cannot see that gate because it is a field on a struct the caller branches on rather than a call in this body",
	"internal/compose/company360/deals.go:closedTotals":                    "the won and lost totals under that same band and that same gate",
	"internal/compose/company360/deals.go:dealsSection":                    "the account's deals band. assemble.go asks auth.Require(a.ctx, \"deal\", ActionRead) before this section and scopeClause carries the row half",
	"internal/compose/companyrollupread.go:closedWonMinorThisQuarter":      "the quarter's closed-won figure under that same three-object gate",
	"internal/compose/companyrollupread.go:weightedPipelineMinor":          "the weighted pipeline the account rollup sums. CompanyHierarchyRollup asks auth.Require for company, deal and activity in a LOOP over datasource.EntityType constants, so the object arrives as `string(object)` and no static reader can resolve it",
	"internal/compose/contact360/commercial.go:leadingDealSeat":            "the leading deal seat on a contact page. commercialSection asks requireRead(ctx, \"deal\") — the package-local wrapper over auth.Require — before reaching it, and the edge bound here is the endpoint conjunction the deal scope alone does not give",
	"internal/compose/meetingbrief/meeting.go":                             "the pre-meeting brief's room statement. scopeFor now asks auth.ReadGranted for each object it scopes, so the deal band's join matches nothing for a caller with no deal grant — the object half added in this change. The census cannot see it because the object arrives as a PARAMETER",
	"internal/compose/network/coveragefacts.go:readDealFacts":              "the deal facts an account's coverage rests on. CompanyCoverageFor asks the deal object gate before the facts are read",
	"internal/compose/weekly/weeklycompare.go:closedInWeek":                "the week's closed figures. weeklyassemble.go asks auth.Require(ctx, \"deal\", ActionRead) before the review is assembled",
	"internal/compose/weekly/weeklycompare.go:openedInWeek":                "the week's opened figures, behind that same gate",
	"internal/compose/weekly/weeklycounts.go:countWeek":                    "the week's counts, behind it too",
	"internal/compose/weekly/weeklycounts.go:readWeekDeals":                "the deals the week's blocks are built from, behind it too",
	"internal/compose/weekly/weeklydealscore.go":                           "the deal-score fragment the weekly scorecard composes, in a package whose entry points ask the deal object gate (weeklyassemble.go, teamreviewstore.go, weeklynarrate.go)",
	"internal/compose/weeklyforecastseam.go:dealLabels":                    "the names beside a forecast movement. Its one caller is the forecast seam, reached from weekly/weeklyoutlook.go and weekly/teamreview.go — both past the deal object gate their package asks — and the row scope here narrows to the deals the reader may open",
	"internal/modules/deals/basecurrencyfreezewrite.go:frozenBaseBefore":   "reads the base-currency amount as it stood before a freeze, so the write can tell a genuine change from a re-freeze. Reached from AdvanceDealTx, whose exported entry points ask the deal object gate",
	"internal/modules/deals/changereviews.go:readAppliedChangeReviews":     "the applied change reviews for a set of deals, behind the review surfaces that ask the deal object gate at their entry",
	"internal/modules/deals/closingoccurrence.go:currentClosingOccurrence": "the closing occurrence a stage move is about, read inside the advance's own transaction past the deal gate its entry points take",
	"internal/modules/deals/correctionimage.go:currentCorrectedValues":     "`to_jsonb(d)` — the before-image a correction audits against, read inside the correcting write. Its callers ask the deal object gate and the image goes into the audit row, not to a caller",
	"internal/modules/deals/deal_singleread.go:readDeal":                   "the shared single-row deal read, and the spine every deal surface goes through — get, create, update, advance, the merge survivor. Each entry point asks auth.Require for the deal object and most take a row probe as well",
	"internal/modules/deals/dealcommercial.go:lockedAcquisitionSource":     "the acquisition source held under the deal's lock, so a write cannot silently change where the deal came from. Inside the gated commercial write",
	"internal/modules/deals/forecasthistory.go:recordForecastMovement":     "records a forecast movement against the deal it belongs to, reading the prior standing so the series carries forward. Reached from the advance, past its own gate",
	"internal/modules/deals/health.go:healthExpectedPace":                  "the expected pace a deal's health is judged against, from the stage history. Its callers are the health reads, each asking the deal object gate at their entry",
	"internal/modules/deals/health.go:healthInputs":                        "the activity and stage inputs the same health verdict rests on, behind the same gated entry points",
	"internal/modules/deals/offer.go:resolveBuyerCompany":                  "reads `company_id` from the deal an offer is being created for, so the offer names the right buyer. Reached from CreateOffer, which asks the offer object gate",
	"internal/modules/deals/offer_dealsync.go:syncDealAmountFromOffer":     "reads the deal's current amount and status so an accepted offer's figures can be synced onto it without overwriting a closed deal. Inside the offer write, past its gate",
	"internal/modules/deals/stageevidencewriters.go:criteriaOfKind":        "the evidence criteria the stage a deal sits in demands, read inside the evidence write past its gate",
	"internal/modules/deals/stageprogressionfacts.go:readDealStanding":     "the deal's stage standing — pipeline, stage, position, semantic — behind ReadStageProgressionFacts, which asks the deal object gate",
	"internal/modules/deals/stagerevert.go:heldWinReason":                  "the won-without-contract reason held on the deal, read so a reversal can restore or clear it. Inside the gated reversal write",
	"internal/modules/deals/stagerevert.go:lockDealForReversal":            "takes the deal's row lock before a stage reversal. It selects `id` only and its callers ask the deal object gate",
	"internal/modules/deals/stagerevert.go:lockReversibleMove":             "reads the stage the reversal is moving back from, under that same lock and behind that same gate",
})

// ruledDealReads: a DISCLOSING read the product has ruled needs no deal grant.
var ruledDealReads = gatekit.Waive(map[string]string{
	"internal/modules/privacy/legalholdlist.go": "the litigation-hold census: one UNION over the five holdable tables, selecting an id and a display name for rows where legal_hold is set. Gated on the retention-policy authority and refused to a non-human principal at its own entry, which is the posture the sibling restricted-records list already takes — a controller asked what a hold is preserving has to be told which records those are, and the grant that governs the retention ladder is the one that governs seeing what overrides it. No field of the record is read beyond its name",
})

// deferredDealReads: a DISCLOSING read that is still ungated, each naming
// the issue that will close it.
var deferredDealReads = gatekit.Waive(map[string]string{})

// notTheDealTable: the read pattern matched something that is not the table.
// Empty for : unlike , the word is not a SQL keyword, and the
// pattern's delimiter set already keeps deal_room and deal_stage_history out.
// notTheDealTable: the read pattern matched something that is not this table.
//
// Empty for `deal`, and the emptiness is a finding rather than an omission:
// unlike `lead`, the word is not also a SQL keyword, and TableReadPattern's
// delimiter set already keeps `deal_room` and `deal_stage_history` out because
// an underscore is not a delimiter.
var notTheDealTable = gatekit.Waive(map[string]string{})

var dealVerdicts = []namedVerdict{
	{"predicate", predicateDealReads},
	{"lifecycle", lifecycleDealReads},
	{"callee-gated", calleeGatedDealReads},
	{"not-the-table", notTheDealTable},
	{"ruled", ruledDealReads},
	{"deferred", deferredDealReads},
}

// wantMinimumGatedDealSites is the floor below the count of sites that
// satisfy the gate today. It exists for the reason every floor in this
// directory does: an extractor that stops recognising SQL finds no sites and
// reports nothing, which is indistinguishable from a clean tree.
const wantMinimumGatedDealSites = 16

var dealReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsDealTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsDealTable(filePath string, file *ast.File) bool {
	return gatekit.FileReadsTable(filePath, file, dealGate.literal)
}

func TestEveryReaderOfTheDealTableCarriesTheObjectGateOrAVerdict(t *testing.T) {
	t.Parallel()
	files := dealReaderScope.Files(t)
	gated := dealGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, site := range dealGate.readSites(parsed, consts.of(t, pkg)) {
			subject := parsed.Path
			if site.function != "" {
				subject += ":" + site.function
			}
			carriesGate := site.holdsGate || callsAGatedHelper(site.calls, gated[pkg])
			if site.function == "" {
				carriesGate = site.holdsGate || dealGate.fileHoldsAGatedFunction(t, parsed, gated[pkg], consts.of(t, pkg))
			}
			verdict := verdictIn(t, subject, dealVerdicts)

			switch {
			case carriesGate && verdict != "":
				t.Errorf("%s carries the deal object gate AND a %s verdict — remove the verdict, it "+
					"now describes code that is gated", subject, verdict)
			case carriesGate:
				satisfied++
			case verdict == "":
				t.Errorf("%s reads the deal table without the object gate and without a verdict.\n"+
					"  Reading a deal discloses the commercial position — the figure, the stage "+
					"and the close date — which is what the deal grant governs.\n"+
					"  Either ask auth.Require(ctx, \"deal\", …) — or auth.ReadGranted where the "+
					"read must degrade rather than refuse — or declare it in "+
					"predicateDealReads / lifecycleDealReads / calleeGatedDealReads / "+
					"ruledDealReads with the reason it needs no gate.\n"+
					"  Left alone it is indistinguishable from a read nobody considered.\n"+
					"  The read: %s", subject, site.sql)
			}
		}
	}

	if satisfied < wantMinimumGatedDealSites {
		t.Errorf("only %d deal reads satisfy the object gate, want at least %d — a literal extractor "+
			"that stopped recognising this tree's SQL would report exactly this, and it reads the "+
			"same as a clean tree", satisfied, wantMinimumGatedDealSites)
	}
	t.Logf("deal reads: %d gated, %d predicate, %d lifecycle, %d callee-gated, %d ruled, %d DEFERRED (still disclosing)",
		satisfied, len(predicateDealReads.Subjects()), len(lifecycleDealReads.Subjects()),
		len(calleeGatedDealReads.Subjects()), len(ruledDealReads.Subjects()), len(deferredDealReads.Subjects()))

	predicateDealReads.AssertAllMatched(t)
	lifecycleDealReads.AssertAllMatched(t)
	calleeGatedDealReads.AssertAllMatched(t)
	notTheDealTable.AssertAllMatched(t)
	ruledDealReads.AssertAllMatched(t)
	deferredDealReads.AssertAllMatched(t)
}
