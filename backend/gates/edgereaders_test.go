// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// `relationship` is a first-class RBAC object, and it is the only join table in
// the schema that is one. The reason is the whole obligation this gate holds:
// an edge discloses the two records it names AS A PAIR, which the grants on
// those two records do not cover. "Who works at Acme" is a fact about the pair.
//
// So every read of that table either passes the edge's own gate, or says which
// kind of read it is that does not need to. This gate asks for that verdict and
// fails a read carrying neither — because a read that quietly answers under the
// endpoints' grants is indistinguishable from one that was considered, and the
// tree has now produced nine of the first kind while a tenth sat one directory
// away doing it correctly.
//
// Why the gate is the fix rather than the helper. platform/auth.EdgeReadScope
// spells the admission once, but nothing forces a new SQL read through it — the
// tree's own precedent is explicit that one spelling holds because a gate
// enforces reaching it (RelationshipEndpointScope's "one spelling" comment, and
// composerowscope_test.go enforcing it). This is that gate.
//
// What it asks, in three derived steps:
//
//   - the SITES are every SQL string literal under internal/ naming the
//     relationship table, resolved from the literals themselves, so a new read
//     inherits the obligation with no edit here;
//   - the OBLIGATION is that the enclosing FUNCTION reaches an edge-gate
//     spelling, resolved transitively across its package;
//   - anything else carries a VERDICT in one of the four declarations below,
//     and which declaration a site sits in IS its verdict.
//
// Per FUNCTION and not per file, which is where it parts company with
// restrictedreaders_test.go's otherwise identical shape. That gate judges a
// whole file because its subject is one obligation every read in the file
// shares; here one file legitimately holds reads with DIFFERENT verdicts —
// company360/graphreads.go gates two and carries a ruling on the third — and a
// file-level answer would let the gated pair vouch for the third, which is
// precisely the hole being closed. A package-level SQL fragment has no function to belong to
// and is judged at file scope, as it is there.

import (
	"go/ast"
	"path"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// edgeTable is the table this census is about, named once so the pattern and
// the failure messages below cannot disagree about the subject.
const edgeTable = "relationship"

// relationshipGate is the subject: the edge table, what counts as its read
// admission, and the row-scope spellings that satisfy only inside the packages
// owning the object's own surface.
//
// EdgeReadScope is the gate seed, because it takes the object gate. The row-half seeds bound WHICH edges and answer nothing
// about whether the caller may read edges at all — alone they are the INVERTED
// form of the defect this gate exists to catch, and a compose read taking the
// conjunction without the gate must not read green. contacts owns the
// relationship surface and gates every store entry point on it; auth is where
// the gate itself lives.
//
// The literal pattern is gatekit's, shared rather than spelled here because a
// matcher that stops seeing this tree's SQL finds nothing to object to and
// reads exactly like a clean tree. Its boundary cases are tested where it lives.
var relationshipGate = objectGate{
	object:        edgeTable,
	literal:       gatekit.TableReadPattern(edgeTable),
	gateSeeds:     []string{"EdgeReadScope"},
	rowHalfSeeds:  []string{"RelationshipEndpointScope", "EnsureRelationshipVisible"},
	rowHalfOwners: []string{"internal/modules/contacts", "internal/platform/auth"},
}

// predicateEdgeReads: the edge appears only inside a JOIN or EXISTS that
// selects or routes records the caller is separately gated on, and nothing
// about the edge reaches the response. The test that puts a read here rather
// than in the gated set: removing the edge condition could only WIDEN the
// result the caller already sees.
//
// The discriminator matters because the obvious wording of it is wrong. "The
// edge's columns are not in the select list" would file the employer filter on
// the contacts list here — and filtering a list by employer answers "who works at
// Acme" one page at a time, which is a STRONGER disclosure than the count on
// the account. It carries the gate, and is not in this set.
var predicateEdgeReads = gatekit.Waive(map[string]string{
	"internal/modules/search/graphcompanyreach.go":                               "the same employment arm as activities/companyscope.go, spelled for the context walk because a module never imports a sibling (ADR-0054) — the two texts are held equal by TestTheAccountReachWalkIsOneAnswer. It carries the same cost for the same reason: the arm is machinery that ANSWERS which activities an account reaches, and it only ever widens that set",
	"internal/modules/activities/companyscope.go":                                "the employment arm of the company row-scope walk, which is the machinery that ANSWERS what a caller may see — gating it on a grant would be circular, and the arm only ever widens the set of activities a company reaches. The cost is that this walk reaches an edge no grant was checked for, on a path whose whole output is a visibility predicate",
	"internal/modules/activities/projectcoverage.go":                             "the stakeholder arm of the project filing-coverage count carries RelationshipEndpointScope, so a seat whose endpoints the caller may not read never pulls an activity into the count; what it does not ask is the relationship OBJECT gate, because no edge column or endpoint pair reaches the caller — only a number does, over their own activity row scope. The cost is that the count weakly reflects that some visible seat exists on the project",
	"internal/modules/activities/lasttouch.go":                                   "the stakeholder arm of the cold-queue selector: the edge decides which QUEUE ENTRIES are stale enough to surface, and no edge column or endpoint pair reaches the caller. Removing the arm would widen the queue, never narrow it. The cost is that a queue entry's presence weakly reflects that some seat exists on some open deal",
	"internal/modules/capture/purgeselect.go:SelectPurgeableContactsTx":          "the deal-stakeholder arm of the purge's rival-holder test: the edge decides whether a contact is somebody the workspace knows INDEPENDENTLY of the purging mailbox, and its only effect is to REFUSE destroying that contact. No edge column and no endpoint pair reaches the caller — a contact id does, and only for a contact whose every address matches the caller's own rule. Removing the arm would widen what is destroyed, never narrow it, so gating it could only expose a contact to destruction that this test exists to protect",
	"internal/modules/contacts/cohortpromote.go:DomainsOwedTheirContacts":        "an EXISTENCE test on the sweep's own selection: it asks whether a contact already holds an employment slot so the backlog pass skips them, and no edge column or endpoint pair reaches any caller — the answer is a company id and a domain string, and only for a company that already holds that domain. Removing the arm would widen what the sweep plants, never narrow it. It runs under the system principal, which is the whole reason it is a sweep: no human naming a company holds authority over contacts they cannot see",
	"internal/modules/deals/championcover.go:withheldSeats":                      "the probe that keeps a champion gap honest, and it is UNGATED on purpose: it asks for exactly the rows the edge admission REFUSES, as `NOT (RelationshipEndpointScope)`, and ChampionCoverFor took the object gate before reaching it — a refused caller returned from championSeats already, so taking it again would be a second Require on one request — it would report \"nobody is carrying this deal\" on a deal whose champion the reader merely cannot see. This asks only WHETHER some seat on the deal is one the edge conjunction refuses — the complement of the visible read rather than a copy of one of its six arms, so a seat hidden by counterparty_company_id is caught as surely as one hidden by contact_id. What reaches the caller is one BIT per deal, and never a contact id, a role, a count or an endpoint pair. The deal ids are the caller's own: every one arrives from the deals module's row-scoped list, and this statement carries no deal predicate of its own because `deal` is an identity table, so a clause here would render true and narrow nothing. The cost is that the bit weakly confirms some seat exists on a deal the caller can already open, which is exactly what the queue has to admit to avoid claiming a deal is unchampioned. Held by TestAChampionTheReaderMayNotSeeIsReportedWithheldRatherThanAbsent and its two siblings in compose/integration",
	"internal/compose/network/accountcoverage.go:countAccountStakeholders":       "the probe that keeps an account's threading verdict honest, ungated on purpose in the same shape as deals/championcover.go:withheldSeats. AccountCoverageFor takes the edge admission before reaching it, so a refused caller returned already and taking it again would be a second Require on one request. It carries the deal and project scopes like the visible half beside it and omits only the CONTACT predicate, so their difference means `a contact I cannot open` rather than `a deal I cannot reach`, so the difference from the visible half is the number withheld — and without that number an account whose contacts are capture-private to another mailbox reports `single_threaded`, which is a permission printed as a fact about the customer. What reaches the caller is one BOOLEAN — coverage is or is not incomplete — never a count, a contact id, a name, a role or an endpoint pair; an exact number would let a reader watch it move and learn that a colleague captured a new contact here. The cost is that the count weakly confirms some seat exists on an account the caller can already open, which is what the verdict has to admit to avoid being wrong. Held by TestAnAccountWithHiddenContactsIsUnknownRatherThanSingleThreaded in compose/integration",
	"internal/compose/network/accountcoverage.go:accountStakeholderEdge":         "renders the predicate both of this file's statements select on and executes nothing itself; its project arm names relationship a second time to reach the account through project_company. Both statements it serves are behind AccountCoverageFor's edge admission",
	"internal/modules/contacts/demote.go":                                        "the guard that refuses demoting a company still carrying live edges — an existence test whose answer is the refusal itself, on a write the caller is already gated for. The cost is that a refused demote confirms edges exist, which the write grant already implies",
	"internal/compose/signalscan_project.go":                                     "the company arm of the quiet-project scan, which runs as PrincipalSystem on a schedule and reads the edge to decide WHERE a signal lands. No caller is present to gate, and the signal it writes is then read under the reader's own signal row scope",
	"internal/modules/contacts/anchorguard.go:refuseIfSoleCompanyOnALiveProject": "the guard that refuses archiving a company which is the ONLY one on a live project — an existence count whose answer IS the refusal, on a write the caller is already gated for. No edge column or endpoint pair reaches the caller, only the number of projects that would be stranded. The cost is that a refused archive confirms projects exist, which the refusal has to say to be actionable",
	"internal/compose/reportprojects.go":                                         "the company arms of the project reports: the edge decides which company a project is GROUPED under and which projects a company filter admits, and the report's own reference scope then masks a company the reader may not open. No edge column reaches the caller — a company id and a count do, over the caller's own project row scope. The cost is that a grouped row weakly reflects that the company is on those projects, which is what the report is FOR",
	"internal/modules/projects/read.go:appendProjectFilters":                     "the company arm of the project list's own filter: the edge decides WHICH projects the list shows for a named company, and no edge column reaches the caller — project rows do, over the caller's own project row scope. Removing the arm would widen the list, never narrow it. The cost is that a listed project weakly reflects that the named company is on it, which is what the filter is FOR",
	"internal/modules/collections/vocab.go":                                      "the project segment's company filter compiles to an EXISTS over the edge — the edge decides which projects a saved view admits, and the rows it yields carry the caller's own project row scope. No edge column reaches the caller",
	"internal/modules/privacy/edgeerasure.go:EdgeBehindErasureBoundary":          "the erasure boundary of a LINK reads the endpoint columns only to ask whether either record the link joins carries a scrub tombstone newer than the audited row. What reaches the caller is one boolean, rendered as the behind_erasure_boundary refusal — no endpoint id, label or pair. It is asked past HistoryServesEntry, whose admission composes auth.EdgeReadScope, so the entry was admitted under the edge gate before this runs, and removing the arm could only WIDEN the reversal it bounds. The cost is that a refusal weakly reflects that one of the link's two ends was erased, which the refusal has to say to be actionable",
	"internal/modules/consent/authorizevalidators.go:validateInvoice":            "the employment (or stakeholder) arm of outbound authorization: the edge decides whether a document the caller already named — an invoice, a contract, a quote — REACHES this recipient, and its only effect is to admit a message that would otherwise fall through to the consent question. No edge column, endpoint id or pair reaches anyone: one boolean does, consumed inside the engine to set a decision row's category. Removing the arm would WIDEN what the engine admits, so gating it could only refuse a real invoice to a real finance contact, and the dispatching worker holds the system principal because no human is present at transmit time. The document id is probed for readability before this runs (refuseUnreadableEvidence, in the same file: a row-scope probe for the deal, the object grant for an invoice or a contract), so a caller cannot name a record they may not open. The cost is that this path learns an employment exists at a company the caller was not separately gated for",
	"internal/modules/consent/authorizevalidators.go:validateContract":           "the employment (or stakeholder) arm of outbound authorization: the edge decides whether a document the caller already named — an invoice, a contract, a quote — REACHES this recipient, and its only effect is to admit a message that would otherwise fall through to the consent question. No edge column, endpoint id or pair reaches anyone: one boolean does, consumed inside the engine to set a decision row's category. Removing the arm would WIDEN what the engine admits, so gating it could only refuse a real invoice to a real finance contact, and the dispatching worker holds the system principal because no human is present at transmit time. The document id is probed for readability before this runs (refuseUnreadableEvidence, in the same file: a row-scope probe for the deal, the object grant for an invoice or a contract), so a caller cannot name a record they may not open. The cost is that this path learns an employment exists at a company the caller was not separately gated for",
	"internal/modules/consent/authorizevalidators.go:validateQuote":              "the employment (or stakeholder) arm of outbound authorization: the edge decides whether a document the caller already named — an invoice, a contract, a quote — REACHES this recipient, and its only effect is to admit a message that would otherwise fall through to the consent question. No edge column, endpoint id or pair reaches anyone: one boolean does, consumed inside the engine to set a decision row's category. Removing the arm would WIDEN what the engine admits, so gating it could only refuse a real invoice to a real finance contact, and the dispatching worker holds the system principal because no human is present at transmit time. The document id is probed for readability before this runs (refuseUnreadableEvidence, in the same file: a row-scope probe for the deal, the object grant for an invoice or a contract), so a caller cannot name a record they may not open. The cost is that this path learns an employment exists at a company the caller was not separately gated for",
	"internal/modules/consent/authorizeevidence.go:liveDealInLinks":              "the deal-stakeholder arm of outbound authorization: the edge decides whether a live opportunity SUPPORTS writing to this recipient, and its only effect is to admit a message that would otherwise fall through to the consent question. No edge column, endpoint id or pair reaches anyone — one boolean does, and it is consumed inside the engine to set a decision row's category. Removing the arm would NARROW what the engine allows, never widen it, which is the reverse of the usual risk here: gating it would refuse correspondence on a deal the dispatching worker cannot see, and the worker holds the system principal precisely because no human is present at transmit time. The cost is that this path learns a seat exists on a deal the caller was not separately gated for, in exchange for not refusing a rep's follow-up on their own opportunity",
	"internal/modules/contacts/companycommitments.go:countCompanyCommitments":    "the count that keeps the account's promise card honest. The scoped read beside it filters on contact scope AND the edge gate, so a promise the caller may not see is simply absent from its rows — and a card built from those alone would show the quiet state on an account that owes something, which is the one thing it must never say. This asks only HOW MANY open promises the account carries, deliberately unscoped, so the two can be compared: fewer admitted than exist marks the page attention_withheld. No endpoint id, name or pair reaches the caller, and no promise body or quote — only the number, and only for a company the caller was already admitted to. Removing the employment arm would widen the count, never narrow it, so a wider count can only make the page MORE willing to admit it is showing less. The cost is that the number weakly reflects that promises exist the reader cannot open, which is exactly what the card has to admit to avoid claiming an account is clear",
	"internal/compose/company360/coverage.go:seatCount":                          "the count that keeps a committee gap honest. deals.Stakeholders applies the contact row scope itself, so a seat whose holder the caller may not see is ABSENT from the slice — and a gap computed from that slice alone would report \"nobody is champion\" on a deal that has one. This asks only HOW MANY live seats the deal carries, deliberately unscoped, so the two can be compared: when they disagree the gap list is suppressed rather than published. No endpoint id, name or pair reaches the caller, only the number, and the caller is already past the deal's own scope clause to have reached this deal at all. The cost is that the number weakly reflects that seats exist the reader cannot list, which is exactly what the page has to say to avoid naming a hole that is not there",
	"internal/modules/contacts/dedupe.go":                                        "the duplicate-candidate scan reads the employer edge to SCORE a pair of contact records the caller can already read both of; the score reaches the caller, the edge does not. The cost is that a high match score weakly reflects a shared employer",
	"internal/modules/integrations/backfillselect.go":                            "the sweep's re-queue test asks whether an employment edge is YOUNGER than a no-identifiers skip — linking an employer is the fix the page asks for, and it lands in relationship while contact.updated_at stands still. An existence test on the sweep's own selection, run under the system principal with no caller to gate: no edge column or endpoint pair reaches anyone, only which contacts the sweep asks the provider about next. Removing the arm would narrow the sweep to contacts fixed via the contact row alone, silently ignoring the page's own advice",
})

// lifecycleEdgeReads: cascades, merges, lawful-processing sweeps, capture and
// enrichment resolution, seeders. Each is gated by the WRITE it belongs to, or
// runs as PrincipalSystem — which auth.Require short-circuits outright
// (platform/auth/rbac.go), so the gate would admit them anyway and the entry
// records why asking was never the point.
//
// The privacy readers are the load-bearing ones. A retention or erasure sweep
// that respected a caller's grants would UNDER-DELETE, which is a worse defect
// than the disclosure this change fixes: the sweep's correctness depends on
// seeing every edge, and it runs as the system principal precisely so it does.
var lifecycleEdgeReads = gatekit.Waive(map[string]string{
	"internal/compose/assuranceseam.go":                                                "the nightly input check's subject assembly, running as the system principal a job binds — auth.Require short-circuits it outright. The edge read is an EXISTS asking whether any economic-buyer seat exists on an open deal; no edge column, endpoint pair or contact id leaves the query — one boolean does, and it reaches readers only through exceptions their own row scope filters on the way out. The cost is that a finding's presence weakly reflects that no such seat exists on a deal the reader can already open",
	"internal/modules/privacy/sarsections.go":                                          "the subject-access export must enumerate every edge naming the data subject, or the export it produces is incomplete — a lawful-processing defect. Runs as the system principal on a request a human already authorised. The cost is that this path reads every edge of one subject with no per-caller gate",
	"internal/modules/privacy/erasure_graph.go":                                        "the erasure walk must reach every edge naming the subject or it leaves data behind that was ordered deleted. Respecting a caller's grants here would under-delete. The cost is unlimited edge reach for the sweep, bounded by its system principal",
	"internal/modules/privacy/retentionselectors.go":                                   "the selector excludes rows still anchored by a live edge, so an edge it cannot see would be deleted while still referenced. The cost is unlimited edge reach in a predicate whose output is a delete bound",
	"internal/compose/company360/roleproposalwrite.go:seatedNow":                       "the buying-role reading's pre-write committee re-read, inside the writing transaction: it asks whether a proposed seat would second an answer a human already gave, and an unseen seat is still an answer — so gating it would let the reading overwrite exactly the seats it may not see. The caller's relationship-create and deal-write authority are both established before the reading begins. No contact id, role or endpoint pair leaves the function, only the decision not to write. The cost is that this path reads every live seat on one deal with no per-caller gate",
	"internal/modules/contacts/employmentsuccession.go:lockContactForEmployment":       "the row lock a stakeholder or employment write takes before it changes the edge, called only from CreateRelationship, UpdateRelationship and ArchiveRelationship, each of which asks the relationship create, update or delete grant and the anchor's update grant at its own entry. The cost is that the lock trusts its callers rather than re-asking",
	"internal/modules/contacts/employmentsuccession.go:promoteLoneSurvivingEmployment": "the successor promotion, running inside the transaction of an archive or a patch that has already asked relationship delete-or-update AND the anchor's update grant, on an edge the caller's own visibility probe admitted. It reads this contact's other employments to answer one question — is exactly one left — and NOTHING leaves the function: no edge column, no endpoint pair, no id, not even a boolean. Its only effect is a flag on a row the same caller may already edit. Gating it on the caller's edge scope would make the answer depend on which of a contact's employments the caller happens to see, so two admins retiring the same job would leave the contact with different employers. The cost is that this path reads every live employment of one contact with no per-caller gate",
	"internal/modules/contacts/relationshipcreate.go:writeRelationshipInTx":            "the edge's own INSERT, plus the current-primary rules that read the contact's other employments to decide which one is current. Both entry points above it — CreateRelationship and CreateRelationshipTx — ask relationship create AND the anchor's update grant before reaching it, and its RETURNING row goes back to that same gated caller. The cost is that the writer trusts its two callers rather than re-asking, the same trade lockContactForEmployment above makes",
	"internal/modules/contacts/merge.go":                                               "the contact merge re-points and archives the loser's edges inside the merge write, gated by that write's own grant. The cost is that merge reaches edges the caller could not have listed",
	"internal/modules/contacts/merge_company.go":                                       "the company merge, same shape and same write grant. The cost is the same",
	"internal/modules/contacts/employmentedge.go":                                      "the shared employment upsert, gated by relationship create at its callers. The cost is none beyond that gate",
	"internal/modules/contacts/providerclaimtargets.go":                                "a purchased employment edge, planted while folding a provider run's answers onto the record. It reads the contact's current-primary slot to decide whether they already have an employer — the same read employmentedge.go above makes, for the same decision — and it runs on the provider hand-off, whose principal is the system one the worker binds. The cost is that the fold resolves one employment slot per subject with no per-caller gate",
	"internal/modules/contacts/projectcompany.go:setCompanyRoleTx":                     "the same statement's own view of the edge it is about to upsert, read so the audit row can say whether this attach created the company link or moved its role, and what the role was. Reached only past auth.EnsureLinkTarget on the company and the project-company write's own grant, and what it discloses is one role, into the image of the write that changed it. The cost is that the upsert reads its own prior row rather than re-asking",
	"internal/modules/contacts/projectstakeholder.go:projectStakeholderEdge":           "the private lookup of the seat a write is about to create or remove, called only from SetProjectStakeholder and RemoveProjectStakeholder, both of which ask the relationship create and delete grants at their own entry. The cost is that the helper trusts its two callers rather than re-asking",
	"internal/modules/contacts/domaintriageresolve.go":                                 "domain triage promotes a captured contact to an employment edge as part of resolving a triage decision — a write gated by its own grant, whose read checks whether the edge it is about to make already exists. The cost is that triage confirms an edge exists before creating it",
	"internal/modules/contacts/enrichment.go":                                          "the enrichment writer reads the employer edge to decide which company a fetched fact belongs to, on a path that runs as a system principal from the enrichment job. The cost is that enrichment resolves an employer with no per-caller gate",
	"internal/modules/contacts/sitecontactfields.go":                                   "site-derived contact fields are matched to the employer edge during capture resolution, as a system principal, before any human reads the result. The cost is the same as enrichment's",
	"internal/modules/contacts/linkedinmatch.go":                                       "profile matching resolves a captured profile against the employer edge during capture, as a system principal. The cost is the same as enrichment's",
	"internal/modules/contacts/companynamepromotion.go":                                "company-name promotion reads the employment edges it is about to re-point when a captured company name is promoted to a record — a write path running as a system principal. The cost is the same as enrichment's",
	"internal/modules/signals/resolver.go":                                             "signal resolution matches an inbound signal to a company through the employment edge, as a system principal on the capture path, before any human is shown the signal. The cost is that resolution reads edges with no per-caller gate",
	"internal/compose/contactautoenrich.go":                                            "the auto-enrich pass resolves a contact's current primary employer to decide which company site may describe them, under an explicit PrincipalSystem actor it binds itself. The cost is that the pass reads one employer edge per candidate with no per-caller gate",
	"internal/compose/captureofflinedemo.go":                                           "the offline capture demo seeds a directory of contacts to write to from an account's edges; it is a development seeder, never a served read. The cost is none — it reaches no caller",
})

// deferredEdgeReads: a DISCLOSING read that is still ungated, each naming the
// issue that will close it.
//
// It is a real verdict and not a quiet exclusion, which is the distinction
// #1831 drew when it refused a bare exemption list: a bare list reads as
// "handled", this reads as "decided, and not yet done". The hole stays
// countable — `go test -run TestEveryReaderOfTheRelationshipTable -v` prints
// the count.
//
// Every reason here names its issue. That is a convention rather than a checked
// rule, deliberately: a parallel map of reasons to police it is exactly what
// TestEveryPackageLevelReasonMapIsAWaiverOrADeclaredFixture refuses, because a
// gate holding its own exceptions to its own standard is how the standards
// diverge. gatekit already refuses a reason that states no cost.
//
// A deferral is a claim that the work is pending, so this set is EMPTY when
// nothing is pending — which is the state it is in now, and the state the
// census was built to reach. It stays declared rather than deleted for the
// reason the census exists at all: the next disclosing read that cannot be
// gated today needs somewhere honest to go, and a contributor who finds no such
// place invents one, or worse, quietly gates a read that should not be.
//
// The five entries this held were the four coverage reads — closed by the
// withheld channel on DealCoverage, so the seats, our side and the findings now
// come back empty AND NAMED instead of gated into a false all-clear — and the
// related-companies read, which turned out to be a ruling rather than a
// deferral and moved to ruledEdgeReads.
var deferredEdgeReads = gatekit.Waive(map[string]string{})

// ruledEdgeReads: a DISCLOSING read the product has ruled needs no edge gate.
//
// A fourth verdict rather than a stretched third, because the three existing
// ones would each be a lie here. Not predicate — removing the edge condition
// would not merely widen what the caller sees. Not lifecycle — it serves a
// human read. And not deferred: a deferral says the work is pending, and
// recording a ruling as one leaves a hole nobody can ever close, since the
// thing it waits for is never going to happen.
//
// A ruling states the sentence it rests on, not who made it. The reason a
// reader needs is why the edge grant does not bear on this read; a name would
// date, and the record of who decided lives in git and the issue.
var ruledEdgeReads = gatekit.Waive(map[string]string{
	"internal/modules/consent/confirmcard.go:confirmCardFor":         "the employer shown to a data subject on their OWN confirm card. RULED to need no edge grant, on the one ground that cannot generalize: the reader IS one endpoint of the edge, so the pair this discloses is \"you, and the company you work for\" — a fact the subject supplied and already knows. There is no principal to gate against either, by design: the surface has no session and the single-use token delivered to that contact's mailbox is the whole of the authority. Gating it would mean inventing an authority for a contact to see their own employment, which is not a thing the RBAC model has or should have",
	"internal/compose/company360/graphreads.go:readRelatedCompanies": "the partner/referral/co-sell edges on the related-companies card. RULED to need no edge grant: crm.yaml states these companies need no grant beyond the company read the endpoint already demands and can never be withheld wholesale, and groups_omitted's enum has no value that could name them. The edge grant exists because an edge discloses its endpoints AS A PAIR — and both endpoints here are companies this endpoint already required the grant for, with no contact named. The cost is that one disclosing read of the table sits outside the rule permanently, and this entry is where that is visible",
})

// wantMinimumGatedSites is the floor below the count of sites that satisfy the
// gate today (twenty, across compose, the modules that read edges, and the
// module that owns the object's own surface).
//
// It exists for the reason composerowscope_test.go's equivalent does: an
// extractor that stops recognising SQL finds no sites and reports nothing,
// which is indistinguishable from a clean tree. The floor sits below the true
// count rather than on it, so removing one read stays an ordinary change and
// only a collapse is a finding.
const wantMinimumGatedSites = 16

// edgeReaderScope is every non-test, non-generated file under internal/ that
// reads the relationship table by name.
var edgeReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsRelationshipTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsRelationshipTable(filePath string, file *ast.File) bool {
	return gatekit.FileReadsTable(filePath, file, relationshipGate.literal)
}

func TestEveryReaderOfTheRelationshipTableCarriesTheEdgeGateOrAVerdict(t *testing.T) {
	t.Parallel()
	files := edgeReaderScope.Files(t)
	gated := relationshipGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, site := range relationshipGate.readSites(parsed, consts.of(t, pkg)) {
			subject := parsed.Path
			if site.function != "" {
				subject += ":" + site.function
			}
			// Its own body first, then the helpers IT CALLS — never its own
			// name. A by-name lookup would let a gated Store.X vouch for an
			// ungated Handlers.X, which is the vouching this gate is
			// per-function precisely to prevent.
			carriesGate := site.holdsGate || callsAGatedHelper(site.calls, gated[pkg])
			if site.function == "" {
				carriesGate = site.holdsGate || relationshipGate.fileHoldsAGatedFunction(t, parsed, gated[pkg], consts.of(t, pkg))
			}
			verdict := verdictIn(t, subject, edgeVerdicts)

			switch {
			case carriesGate && verdict != "":
				// A satisfied site that is ALSO waived is the way this census
				// silently stops counting down: a deferred read that later gets
				// its gate keeps matching its waiver for ever, and the hole
				// reads as open when it is closed.
				t.Errorf("%s carries the edge gate AND a %s verdict — remove the verdict, it now "+
					"describes code that is gated", subject, verdict)
			case carriesGate:
				satisfied++
			case verdict == "":
				t.Errorf("%s reads the relationship table without the edge gate and without a verdict.\n"+
					"  Reading an edge discloses its endpoints AS A PAIR, which is what relationship.read "+
					"governs — the endpoints' own grants do not cover it.\n"+
					"  Either call auth.EdgeReadScope, or declare the read in predicateEdgeReads / "+
					"lifecycleEdgeReads / deferredEdgeReads with the reason it needs no gate.\n"+
					"  Left alone it is indistinguishable from a read nobody considered.\n"+
					"  The read: %s", subject, site.sql)
			}
		}
	}

	if satisfied < wantMinimumGatedSites {
		t.Errorf("only %d relationship reads satisfy the edge gate, want at least %d — a literal "+
			"extractor that stopped recognising this tree's SQL would report exactly this, and it "+
			"reads the same as a clean tree", satisfied, wantMinimumGatedSites)
	}
	t.Logf("edge reads: %d gated, %d predicate, %d lifecycle, %d ruled, %d DEFERRED (still disclosing)",
		satisfied, len(predicateEdgeReads.Subjects()), len(lifecycleEdgeReads.Subjects()),
		len(ruledEdgeReads.Subjects()), len(deferredEdgeReads.Subjects()))

	predicateEdgeReads.AssertAllMatched(t)
	lifecycleEdgeReads.AssertAllMatched(t)
	ruledEdgeReads.AssertAllMatched(t)
	deferredEdgeReads.AssertAllMatched(t)
}

// edgeVerdicts are the four declarations, in the order a reader meets them
// above. Named as one list because "which declaration a read sits in IS its
// verdict" is only true if every declaration is asked — a set left off this
// list would silently stop counting as a verdict and start reading as a
// finding, which is the one direction this census must not fail in.
var edgeVerdicts = []namedVerdict{
	{"predicate", predicateEdgeReads},
	{"lifecycle", lifecycleEdgeReads},
	{"ruled", ruledEdgeReads},
	{"deferred", deferredEdgeReads},
}
