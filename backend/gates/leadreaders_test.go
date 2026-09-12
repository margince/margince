// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// `lead` is an RBAC object, and until this gate nothing held the object half of
// that anywhere outside the module that owns it.
//
// A lead is somebody who has NOT agreed to be a customer: a form fill, a list
// purchase, a conference badge scan. The grant exists because a seat with no
// business in the top of the funnel should not be able to read one — names,
// addresses, employers and a score saying how promising the company thinks
// they are.
//
// Two gates cover the compose tier and both say in their own words that they do
// not cover this: rbacgate_test.go judges only exported methods on a module's
// *Store or *Service, and composerowscope_test.go covers ROW scope and refuses
// to count object admission as satisfaction. Between them the object half had
// no fitness function outside internal/modules/contacts.
//
// So every read of the lead table either asks the object gate, or says which
// kind of read it is that does not need to. The shape, the discriminator and
// the three verdicts are edgereaders_test.go's — this is the second census over
// objectGate, and the walk is shared rather than copied.

import (
	"go/ast"
	"path"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// leadTable is the table this census is about. Both the pattern and the failure
// messages below read it, so neither can drift from the subject.
const leadTable = "lead"

// leadGate is the subject. No platform spelling gateSeeds it: unlike the edge,
// a lead has no bespoke admission helper — `auth.Require(ctx, "lead", …)` IS
// the object gate, and objectGate reads that off the call.
//
// No row-half allowance either, and that is a finding about the tree rather
// than an omission: contacts asks auth.Require at its lead entry points, so the
// transitive resolution reaches the object gate itself and never needs the row
// clause to stand in for it.
var leadGate = objectGate{
	object:  leadTable,
	literal: gatekit.TableReadPattern(leadTable),
}

// predicateLeadReads: the lead table appears only inside a JOIN or EXISTS that
// selects or routes records the caller is separately gated on, and nothing
// about the lead reaches the response. The test that puts a read here rather
// than in the gated set: removing the lead condition could only WIDEN the
// result the caller already sees.
var predicateLeadReads = gatekit.Waive(map[string]string{
	"internal/modules/activities/responsemetrics.go":                   "the lead arm of \"is this a SALES thread\" in the first-response metrics: `%[3]s` renders workingLeadPredicate — a LIFECYCLE test (new/contacted/engaged, unarchived) and not a row-scope clause — so the arm only decides which inbound activities are counted. What reaches the caller is a count and a median over their own activity scope, never a lead column. Removing the arm would widen the set measured, never narrow it",
	"internal/modules/activities/waitingsql.go":                        "two arms, both off the GATED link join: the same workingLeadPredicate sales test as responsemetrics.go, and the ownership walk's `LEFT JOIN lead ownerLead`, whose only projection is owner_id into a four-way COALESCE answering who owns a waiting thread. A USER id, not lead content, and only for threads the caller's own activity scope already admits. Removing either arm widens the worklist or leaves a thread unowned; neither narrows what a caller may see",
	"internal/modules/activities/lasttouch.go":                         "the lead arm of the cold-queue selector, the twin of the stakeholder arm beside it that edgereaders_test.go files the same way: the lead's status and age decide which QUEUE ENTRIES are stale enough to surface, and no lead column reaches the caller. Removing the arm would widen the queue, never narrow it. The cost is that a queue entry's presence weakly reflects that a lead of the caller's is still in the working part of its lifecycle",
	"internal/modules/consent/gate.go:grantedForLead":                  "the lead arm of the consent question, asked of an address the CALLER ALREADY NAMED as the recipient of a send. One boolean leaves it — is there a granted, DOI-confirmed purpose for this address — consumed inside the engine to authorize or refuse the message. No lead id, name, title or company reaches anyone. Removing the arm would refuse a lawful send rather than widen one, which is the reverse of the usual risk here, and the dispatching worker holds the system principal because no human is present at transmit time",
	"internal/modules/consent/authorizelead.go:resolveLead":            "names the one live lead behind that same caller-supplied address, so the grant above can be looked up against a subject. TWO live leads answer `found=false` rather than a guess, which is the whole point of the LIMIT 2: a silent pick would attribute one subject's consent to another. The id it resolves is consumed inside the engine; what the caller is told is authorized or not, and why",
	"internal/modules/consent/withdrawalmint.go:bindWithdrawalSubject": "stamps the subject a one-click unsubscribe link acts for, resolved from the address the send is going TO. A contact wins over a lead, and ambiguity leaves the credential unbound rather than stamping one subject's id on another's opt-out link — the credential still works, because it names the address. Nothing about the lead reaches the recipient or the sender: the token is opaque, and the mint runs on the dispatch path under the system principal",
})

// lifecycleLeadReads: cascades, merges, lawful-processing sweeps, capture and
// enrichment resolution, seeders. Each is gated by the WRITE it belongs to, or
// runs as PrincipalSystem — which auth.Require short-circuits outright
// (platform/auth/rbac.go), so the gate would admit them anyway and the entry
// records why asking was never the point.
var lifecycleLeadReads = gatekit.Waive(map[string]string{
	"internal/modules/privacy/sar.go:subjectReach":                          "the subject-access export must enumerate every lead that was promoted into this contact, or into a record later merged into them, or the export it produces is incomplete — a lawful-processing defect. It reads ids and nothing else, to reach the lead-keyed proof A6 leaves on the lead rather than on the survivor. Runs as the system principal on a request a human already authorised. The cost is that this path resolves one subject's lead twins with no per-caller gate",
	"internal/modules/privacy/sarsections.go:sarRecordSections":             "the lead record AS the export: every column of it is what Art. 15 owes the data subject about themselves, so a grant that narrowed it would make the answer wrong rather than safer. Runs as the system principal on a request a human already authorised, and the rows it reads are already bounded to the subject's own lead twins. The cost is unlimited lead reach for the export, bounded by that subject binding",
	"internal/modules/privacy/sarsections.go:sarProvenanceSections":         "the handoff provenance arm of the same export, reaching the subject's leads through the same promoted_contact_id join for the same reason. The cost is the same",
	"internal/modules/privacy/erasuretimeline.go:deleteSubjectHandoffs":     "the erasure's handoff arm names sdr_handoff_event and sdr_handoff explicitly rather than leaving them to a cascade, so both this census and the next reader auditing what Art. 17 destroys can see them. The lead subquery only RESOLVES which handoffs belong to the subject, by promotion; no lead column leaves the statement and the statement is a DELETE. Respecting a caller's grants here would under-delete, which is a worse defect than the disclosure this census fixes",
	"internal/modules/privacy/retention_leadrecord.go:anonymizeLead":        "the retention scrub reads the lead's email FOR UPDATE so a standing objection is detached onto the address it applies to before the UPDATE beside it nulls that address — read after the scrub and the objection is orphaned, and a re-capture from the same address comes back mailable. Runs as the system principal on the retention sweep; the address never leaves the transaction that erases it. The cost is that the sweep reads one lead's address with no per-caller gate, which is the whole of what makes the erasure durable",
	"internal/modules/privacy/erasure_selectors.go:notTransitivelyHeld":     "the legal-hold exclusion reads lead.legal_hold to REFUSE erasing an activity still held through a lead. Its only effect is to keep a row, never to disclose one: no lead column leaves the predicate and the statement it joins is an erase bound. A selector that could not see a lead would delete a row under hold, which is the failure this arm exists to prevent",
	"internal/modules/privacy/retentionrestricted.go:notHeldThroughAnyLink": "the same legal-hold exclusion, spelled for the retention selectors: it asks whether a linked lead holds the row and, if so, leaves it alone. Same shape, same cost — a lead it could not see would be a row deleted under hold",
	"internal/platform/database/storekit/leadidentity.go:liveLeadBy":        "the exclusive-key dedupe probe behind LiveLeadByEmail and LiveLeadByLinkedInURL: it answers whether a live lead already holds this address or profile, so a write can refuse a duplicate rather than mint one. Its predicate is a fixed literal chosen by the two exported wrappers and never caller text. Both callers are writes — contacts' gated lead create and update paths, and capture's lead sink under the system principal — and what leaves is an id that becomes a conflict, never a lead column",
	"internal/modules/activities/followupownership.go:leadOwner":            "reads the lead's current owner so the follow-up task it owns can be reassigned to match. It is a workflow handler answering lead.updated on the bus, under the principal the workflow runner binds, and the write it leads to is on this module's OWN table. A direct read rather than a call into contacts because a module never imports a sibling",
	"internal/modules/search/embedgen.go":                                   "the per-entity source text the embedding lane indexes, mirroring each record's search_tsv columns so the vector and lexical lanes agree about one text. It runs on the indexing path under the system principal, and what it produces is a vector on the index row; the SEARCH that later reads that row carries the caller's own scope. A grant asked here would leave a lead unindexed for everyone rather than hidden from one reader",
	"internal/modules/privacy/retentionselectors.go":                        "the retention selectors themselves, including `lead/unconverted`: they select the IDS a retention rule is due to act on, under the system principal on the sweep's schedule. No lead column reaches any caller — the output is a delete or scrub bound — and a selector that respected a caller's grants would under-delete, which is the defect the sweep exists to prevent",
})

// calleeGatedLeadReads: a private helper inside the module that OWNS the lead
// object, whose every caller asks the object gate at the store entry point
// above it.
//
// A verdict of its own rather than folded into lifecycle, which is where
// edgereaders_test.go puts the same shape. The edge's helpers are cascades and
// sweeps and the word fits them; the lead surface is mostly one store, and
// calling a plain single-row read "lifecycle" would empty that word of meaning
// in the census where it matters most.
//
// What each entry has to establish, and what the reason therefore names: EVERY
// caller of this function asks auth.Require(ctx, "lead", …). One that does not
// makes the entry false, and the entry is where a reviewer checks it — this
// census resolves gates FORWARD (a function that reaches a gate) and cannot
// resolve them caller-ward, because the call graph is by NAME and a gated
// Store.X would then vouch for an ungated Handlers.X. rbacgate_test.go's header
// records being bitten by exactly that.
var calleeGatedLeadReads = gatekit.Waive(map[string]string{
	"internal/modules/contacts/lead_read.go:readLead":                          "the shared single-row lead read, and the spine every lead surface goes through. Its eight callers are DemoteLead, PromoteLead and its preview, MergeLead, CreateLead/CreateLeadTx, UpdateLead and GetLead — each of which asks auth.Require for the lead object at its own entry, and most of which take a row probe as well. The cost is that the spine trusts its callers rather than re-asking eight times in one transaction",
	"internal/modules/contacts/demote.go:promotedContactOf":                    "reads which contact a lead was promoted into, so DemoteLead can lock that contact BEFORE the lead — the order MergeContact takes, and taking it in the other order is the whole of a deadlock. Its one caller asks auth.Require(lead, update) and auth.EnsureWritable on the lead before reaching it, which its own doc records as owed one frame earlier precisely because the lock moved one frame earlier. No lead column beyond that pointer leaves the function",
	"internal/modules/contacts/demote.go:isSharedByOthers":                     "the guard that refuses archiving a promoted contact something else still depends on: it asks whether another lead points at the same contact, and its answer is the refusal. Reached only through unwindContact from DemoteLead, past the lead's own Require and EnsureWritable and the contact's. No lead column reaches the caller, only a boolean, and that boolean only ever KEEPS a record",
	"internal/modules/contacts/lead.go:replayedLead":                           "the source-key idempotency probe: a create carrying the same source_system/source_id answers the standing lead instead of minting a second. It takes the ROW half itself — auth.VisibleTo on the lead it found, answering ErrConflict rather than the row when the caller may not see it — and its two callers, CreateLead and CreateLeadTx, each ask auth.Require(lead, create) first. The census cannot see VisibleTo because it is not Require-shaped, which is what this entry records",
	"internal/modules/contacts/leaddedupe.go:fuzzyLead":                        "the near-match scan a lead write owes its review queue: it reads live leads to find the one this write reads like. Reached only through recordLeadNearMatch, from CreateLead, CreateLeadTx and UpdateLead, each of which asks auth.Require for the lead object at its entry. What it produces is a review row inside the caller's own transaction, not a response",
	"internal/modules/contacts/leadmerge.go:readLeadMergeState":                "one end of a lead merge, passed to mergePair as a function value from mergeLeadTx. Its only caller is MergeLead, which asks auth.Require(lead, update) and then takes the pair lock. The archived branch answers the merge pointer and ErrNotFound, which is what makes a merged-away id resolve to where it went",
	"internal/modules/contacts/leadrecompute.go:recomputeLeadScoreTx":          "the shared §3 recompute inside an open transaction. Its three callers — RecomputeLeadScore, SetLeadManualSignal and UpdateLead's override-clear path — each ask auth.Require(lead, update) and take their own row probe; the doc on RecomputeLeadScore says why the LIVE probe is at the entry point and not here. Nothing leaves the function but a written score",
	"internal/modules/contacts/leadscorehistory.go:appendOverrideScoreHistory": "records a Commercial Judgement override as its own point in the score series, reading the previous entry so the machine numbers are carried forward rather than re-derived. Its one caller is UpdateLead, past auth.Require(lead, update) and the lead's own row probe. Its only effect is a history row on the lead the caller just changed",
	"internal/modules/contacts/leadrouting.go:ownerCapacity":                   "counts each candidate owner's open lead load so routing can place work under the cap. Reached only from RouteLead, which asks auth.Require(lead, update) and holds the routing advisory lock. What it answers is a number per USER and never a lead column — and the eligibility half deliberately says what auth.EnsureAssignee says on the manual path, so the machine cannot place work a human could not have placed",
	"internal/modules/contacts/leadsla.go:recordFirstResponseTx":               "the one spelling of what counts as a lead's first response, shared by the outbox subscriber and the breach scan so the two cannot disagree about it. It takes the ROW half itself (auth.EnsureWritableLive on the lead) and its named caller RecordLeadFirstResponse asks auth.Require(lead, update); the breach scan reaches it holding the lead row it is about to judge. It reads one timestamp, to refuse moving a stamp already set",
	"internal/modules/automation/automations_preview.go:leadPreviewDefs":       "the catalog entries whose blast-radius preview ranges over the lead table, including the count of leads created in the window. resolvePreviewRecipe asks auth.Require(ctx, def.table, principal.ActionRead) before running any of them, and def.table is \"lead\" for exactly these — the object arrives as a struct field, which is why no static reader can see the gate from the definition",
	"internal/modules/contacts/mergeface.go:readLeadFaces":                     "the label and detail line a merge card shows for each side of a pair. Its only caller is DescribeForMerge, which asks auth.Require(entityType, read) — the object name arrives as a parameter, which is why no static reader can see the gate from here — and then narrows the ids through auth.VisibleSubset, so a lead the caller cannot see is absent before this runs",
})

// ruledLeadReads: a DISCLOSING read the product has ruled needs no lead grant.
var ruledLeadReads = gatekit.Waive(map[string]string{})

// notTheLeadTable: the read pattern matched SQL's OWN `lead()` window function
// rather than the table named lead.
//
// gatekit.TableReadPattern counts an opening paren as a delimiter on purpose —
// `FROM rollup($1)` is a read of the rollup, and a census that stopped seeing a
// relation the day it became set-returning would be under-recognising. That
// same rule makes `extract(epoch FROM lead(h.changed_at) OVER …)` look like a
// read, and only for a table whose name is also a SQL keyword.
//
// Declared rather than fixed in gatekit: narrowing the shared pattern to refuse
// `FROM <name>(` would cost every other census the set-returning case, to spare
// one table one entry. Over-recognition answered by a named declaration is the
// safe direction; the alternative is a pattern that reads green over a real
// read somewhere else.
var notTheLeadTable = gatekit.Waive(map[string]string{
	"internal/modules/deals/closedatesweep.go:stageVelocityDays": "`extract(epoch FROM lead(h.changed_at) OVER (PARTITION BY h.deal_id ORDER BY h.changed_at, h.id) - h.changed_at)` — the window function that reads the NEXT row's value, measuring how long a deal sat in each stage. The statement never names the lead table",
})

// deferredLeadReads: a DISCLOSING read that is still ungated, each naming the
// issue that will close it.
var deferredLeadReads = gatekit.Waive(map[string]string{
	"internal/compose/weekly/weeklycounts.go:countWeekLeads": "the week's routed / answered-in-target / breached counts. It takes the ROW half (auth.ScopeClauseFor on lead) and never the object half, so a seat whose role grants no lead read is still answered lead figures for its own rows. Deferred rather than gated because the answer turns on what a weekly review SAYS to such a seat: failing the whole review is fail-closed and breaks a report whose other blocks are fine, and zeroed counts read as failure at work nobody asked of them — which this file's own doc names as the thing it must not print. #5466",
	"internal/compose/weekly/weeklyscorecard.go:hasLeads":    "whether the rep carried a funnel at all, the presence question the lead block hangs off. Same row-half-only shape and the same open question as countWeekLeads beside it. #5466",
	"internal/compose/weekly/weeklyscorecard.go:scoreLeads":  "the lead block itself. It already models the absent case — an absent block IS the answer for a rep who carried no leads — so it is the one of the three a refusal could be folded into cleanly; it is deferred with the other two because a review whose block is omitted and whose counts are not would be worse than either. #5466",
})

var leadVerdicts = []namedVerdict{
	{"predicate", predicateLeadReads},
	{"lifecycle", lifecycleLeadReads},
	{"callee-gated", calleeGatedLeadReads},
	{"not-the-table", notTheLeadTable},
	{"ruled", ruledLeadReads},
	{"deferred", deferredLeadReads},
}

// wantMinimumGatedLeadSites is the floor below the count of sites that satisfy
// the gate today. It exists for the reason every floor in this directory does:
// an extractor that stops recognising SQL finds no sites and reports nothing,
// which is indistinguishable from a clean tree.
const wantMinimumGatedLeadSites = 1

var leadReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsLeadTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsLeadTable(filePath string, file *ast.File) bool {
	return gatekit.FileReadsTable(filePath, file, leadGate.literal)
}

func TestEveryReaderOfTheLeadTableCarriesTheObjectGateOrAVerdict(t *testing.T) {
	t.Parallel()
	files := leadReaderScope.Files(t)
	gated := leadGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, site := range leadGate.readSites(parsed, consts.of(t, pkg)) {
			subject := parsed.Path
			if site.function != "" {
				subject += ":" + site.function
			}
			carriesGate := site.holdsGate || callsAGatedHelper(site.calls, gated[pkg])
			if site.function == "" {
				carriesGate = site.holdsGate || leadGate.fileHoldsAGatedFunction(t, parsed, gated[pkg], consts.of(t, pkg))
			}
			verdict := verdictIn(t, subject, leadVerdicts)

			switch {
			case carriesGate && verdict != "":
				t.Errorf("%s carries the lead object gate AND a %s verdict — remove the verdict, it "+
					"now describes code that is gated", subject, verdict)
			case carriesGate:
				satisfied++
			case verdict == "":
				t.Errorf("%s reads the lead table without the object gate and without a verdict.\n"+
					"  Reading a lead discloses somebody who has not agreed to be a customer, "+
					"which is what the lead grant governs.\n"+
					"  Either ask auth.Require(ctx, \"lead\", …), or declare the read in "+
					"predicateLeadReads / lifecycleLeadReads / calleeGatedLeadReads / ruledLeadReads "+
					"with the reason it "+
					"needs no gate.\n"+
					"  Left alone it is indistinguishable from a read nobody considered.\n"+
					"  The read: %s", subject, site.sql)
			}
		}
	}

	if satisfied < wantMinimumGatedLeadSites {
		t.Errorf("only %d lead reads satisfy the object gate, want at least %d — a literal extractor "+
			"that stopped recognising this tree's SQL would report exactly this, and it reads the "+
			"same as a clean tree", satisfied, wantMinimumGatedLeadSites)
	}
	t.Logf("lead reads: %d gated, %d predicate, %d lifecycle, %d callee-gated, %d ruled, %d DEFERRED (still disclosing)",
		satisfied, len(predicateLeadReads.Subjects()), len(lifecycleLeadReads.Subjects()),
		len(calleeGatedLeadReads.Subjects()), len(ruledLeadReads.Subjects()), len(deferredLeadReads.Subjects()))

	predicateLeadReads.AssertAllMatched(t)
	lifecycleLeadReads.AssertAllMatched(t)
	calleeGatedLeadReads.AssertAllMatched(t)
	notTheLeadTable.AssertAllMatched(t)
	ruledLeadReads.AssertAllMatched(t)
	deferredLeadReads.AssertAllMatched(t)
}
