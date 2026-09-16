// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// `company` is an RBAC object, and until this gate nothing held the object half
// of that anywhere outside the module that owns it.
//
// A company is the account itself — who the installation sells to, what stage
// that relationship is at, which domains and legal entities belong to it. The
// grant exists because a seat with no business on the account side should not
// be handed the customer list, and because a company name is the one field that
// turns every other record into an identifiable one.
//
// Two gates cover the compose tier and both say in their own words that they do
// not cover this: rbacgate_test.go judges only exported methods on a module's
// *Store or *Service, and composerowscope_test.go covers ROW scope and refuses
// to count object admission as satisfaction. Between them the object half had
// no fitness function outside internal/modules/contacts.
//
// The shape is edgereaders_test.go's and the walk is objectGate's, shared with
// leadreaders_test.go rather than copied. What `company` added to that walk is
// the BOOLEAN spelling of the object gate: four hand-rolled copies of
// "may this caller read this object at all" sat in this tree answering yes or
// no rather than admitting or refusing, because the reads they guard must
// degrade instead of failing. They are auth.ReadGranted now, and the walk
// recognises it.

import (
	"go/ast"
	"path"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// companyTable is the table this census is about. Both the pattern and the
// failure messages below read it, so neither can drift from the subject.
const companyTable = "company"

// companyGate is the subject. No platform spelling gateSeeds it: like the lead,
// a company has no bespoke admission helper — auth.Require(ctx, "company", …)
// and auth.ReadGranted(ctx, "company") ARE the object gate, and objectGate
// reads both off the call.
//
// No row-half allowance either. contacts asks the object gate at its company
// entry points, so the transitive resolution reaches the gate itself and never
// needs the row clause to stand in for it — which matters more here than
// anywhere, because a row clause standing in for the object half is the exact
// defect four of the reads below turned out to be.
var companyGate = objectGate{
	object:  companyTable,
	literal: gatekit.TableReadPattern(companyTable),
}

// predicateCompanyReads: the company table appears only inside a JOIN or EXISTS
// that selects or routes records the caller is separately gated on, and nothing
// about the company reaches the response. The test that puts a read here rather
// than in the gated set: removing the company condition could only WIDEN the
// result the caller already sees.
var predicateCompanyReads = gatekit.Waive(map[string]string{
	"internal/modules/privacy/erasure_selectors.go:notTransitivelyHeld":     "the legal-hold exclusion reads company.legal_hold to REFUSE erasing an activity still held through an account. Its only effect is to keep a row, never to disclose one: no company column leaves the predicate and the statement it joins is an erase bound. A selector that could not see a company would delete a row under hold, which is the failure this arm exists to prevent",
	"internal/modules/privacy/retentionrestricted.go:notHeldThroughAnyLink": "the same legal-hold exclusion, spelled for the retention selectors: it asks whether a linked company holds the row and, if so, leaves it alone. Same shape, same cost — a company it could not see would be a row deleted under hold",
	"internal/modules/activities/lasttouch.go:lastTouchCandidateQuery":      "the account arm of the cold-queue selector, the twin of the deal and lead arms beside it that the other censuses file the same way: the company's liveness and age decide which QUEUE ENTRIES are stale enough to surface, and no company column reaches the caller. Removing the arm would widen the queue, never narrow it. The cost is that a queue entry's presence weakly reflects that an account of the caller's still carries an open deal",
	"internal/modules/activities/waitingsql.go":                             "the account arm of the same worklist, off the GATED link join: it decides which waiting threads belong to an account with open business, and what reaches the caller is the thread they already hold activity scope on. Removing the arm widens the worklist rather than narrowing it",
	"internal/modules/deals/deal_read.go:partnerAttributionFilterClause":    "the partner-attribution filter on a DEAL list: it narrows deals to those whose partner company the caller may see. The company appears only inside an EXISTS and no column of it is selected — what the caller gets back is deals, under the deal grant they already hold. Removing the clause would show more deals, never more companies",
	"internal/modules/contracts/visibility.go:VisibleClause":                "the contract visibility predicate. A contract hangs off a deal or, failing that, an account, so the clause asks whether the caller may see one of the two and admits the CONTRACT on that basis. Every column it selects belongs to the contract; the company is reached only by EXISTS. Removing the company arm would widen the contract list",
	"internal/modules/finance/summary.go:companyExists":                     "the existence probe behind the finance summary, and its own doc says why it is there: under row_scope=all auth.EnsureVisible skips the query, so an id naming NO company would be answered with a summary of the workspace's provider rather than a 404. The probe makes a made-up id answer the way a hidden one does. It returns an error or nil and never a company column, on an id the caller supplied",
	"internal/compose/company360/graphourside.go:readAccountOwner":          "reads the account OWNER — a user id and display name — joining company only to reach owner_id. The subject of the read is a colleague, under the contact and activity grants readOurSide asks one frame above; no company column is selected. Removing the join would leave the graph without an owner, never with more account data",
	"internal/compose/company360/contacts.go:contactIdentity":               "the roster's contact cards. The company is joined only to decide whether a PURCHASED employment claim is about this account — the precedence rule that stops \"VP Sales, Globex\" being rendered on Acme's roster — and what is selected is the contact's own name, title and email. Removing the match would render a false employment assertion, which is more disclosure rather than less",
	"internal/modules/contacts/employmentedge.go:companyIsLive":             "the re-check under the row's own lock, between a selector's read in an earlier transaction and the write that acts on it: an archive or a merge in between would otherwise attach a domain's contacts to a record no read returns. One boolean leaves it, inside the writing transaction",
	"internal/modules/contacts/providerclaimtargets.go:companyByDomain":     "resolves a bought employer's domain to the account it belongs to, so a provider claim can plant an employment edge against an existing record rather than minting a duplicate. Its argument is the claim's own domain and what leaves is an id that becomes an edge; no company column reaches any caller",
	"internal/modules/contacts/linkedincompanyplace.go:companyKeys":         "the name index a LinkedIn ghost is placed against. It takes the company ROW scope already — the pass runs under the ghost owner's own authority — and what leaves the call is a placement and a COUNT, read back later through the gated MyLinkedInReach. The cost is that a member with no company grant still has their network placed against accounts; what they are shown of it is gated where it is shown",
	"internal/compose/csvemployer.go:employerIndex":                         "the employer index a CSV import matches an imported contact's employer text against, row-scoped so a company the caller cannot see contributes neither a match nor an ambiguity. What the import reports is which row it linked; the index itself is consumed inside the write. The cost is that an import can link to an account whose name the caller could not have listed — bounded by the row scope it does take",
	"internal/platform/auth/signalscope.go:SignalScopeClause":               "the signal row-scope clause itself, which resolves a signal about a company to the company's own visibility. It is a clause, not a read: it only ever narrows the signals a caller is shown, and it is the platform helper the signal reads compose rather than a reader of its own",
	"internal/modules/consent/confirmcard.go:confirmCardFor":                "the card a data subject is shown of THEIR OWN record, behind a confirm link, so they can see what the installation holds before confirming it. The employer name is one line of that — their own employment, reached through the live relationship. The caller is the subject holding a bearer token that proves their mailbox and no CRM grant at all, so a company grant is not a question that can be asked of them; what bounds the read is that it resolves from the token's own contact",
	"internal/compose/reportperiod.go:winLossSpec":                          "the win/loss report's join to the account, LEFT so a deal with no company still appears. The report's rows are DEALS and its figures are deal figures; the company is joined to group and label them, under the deal and report grants the spec's own runner asks. Removing the join would drop a grouping, never add a row",
	"internal/compose/signalscan.go:scanGhostedThreads":                     "the ghosted-thread scan, which asks which accounts have gone quiet so a signal can be raised. It runs on the scan's schedule under the scan principal and what it writes is a signal row; the signal is then read through the signal grant, which SignalScopeClause above resolves back to the company's own visibility",
	"internal/compose/signalproposals.go:readContradictions":                "the contradiction pass: it reads an account's lifecycle beside the open signals that disagree with it, so a proposal can be raised or withdrawn. It runs under the proposal executor's own principal and what leaves is a proposal row, gated where a human is shown it",
	"internal/compose/signalextractrule.go":                                 "the visibility roll-up inside the conversation fold the thread extraction reads: a company's visibility and owner_id decide whether a thread counts as shared or as one owner's, so the extraction can leave a private thread alone. No company column is selected and what the statement produces is a thread key. Removing the arm would treat a private thread as shared, which is more disclosure rather than less",
	"internal/compose/contactautoenrich.go:searchTerms":                     "the search terms an auto-enrichment lookup is built from — the contact's name and, where it exists, their employer's display name, so a provider is asked about the right human being. The terms are consumed by the provider call inside the enrichment; what comes back is written to the contact under the enrichment's own gate",
	"internal/modules/signals/resolver.go:matchCandidates":                  "resolves a signal's company_domain to the account ids it could be about, so the signal can be bound to one. Only ids are selected, from company_domain joined to company for liveness, and the binding is what a caller later sees — through the signal grant",
})

// lifecycleCompanyReads: cascades, merges, sweeps, capture and enrichment
// resolution, seeders. Each is gated by the WRITE it belongs to, or runs as
// PrincipalSystem — which auth.Require short-circuits outright
// (platform/auth/rbac.go), so the gate would admit them anyway and the entry
// records why asking was never the point.
var lifecycleCompanyReads = gatekit.Waive(map[string]string{
	"internal/modules/privacy/erasure_graph.go:deleteSubjectLinkedInGhosts":      "the erasure's ghost arm names the company join explicitly rather than leaving it to a cascade, so this census and the next reader auditing what Art. 17 destroys can both see it. The company only RESOLVES which ghosts belong to the subject; no company column leaves the statement and the statement is a DELETE. Respecting a caller's grants here would under-delete",
	"internal/modules/privacy/retentionselectors.go":                             "the retention selectors, which select the IDS a retention rule is due to act on under the system principal on the sweep's schedule. No company column reaches any caller — the output is a delete or scrub bound — and a selector that respected a caller's grants would under-delete, which is the defect the sweep exists to prevent",
	"internal/modules/search/embedgen.go":                                        "the per-entity source text the embedding lane indexes, mirroring each record's search_tsv columns so the vector and lexical lanes agree about one text. It runs on the indexing path under the system principal, and what it produces is a vector on the index row; the SEARCH that later reads that row carries the caller's own scope. A grant asked here would leave an account unindexed for everyone rather than hidden from one reader",
	"internal/modules/contacts/cohortpromote.go:DomainsOwedTheirContacts":        "the domain-backlog selector: which accounts have a domain whose contacts carry no employment edge yet. It runs from the link-reconcile sweep under that worker's system context, and what it answers is work to do — an id and a domain — consumed by AttachDomainBacklog in the same sweep",
	"internal/modules/contacts/companylifecycle.go:SetCompanyLifecycleTx":        "the lifecycle stage move, reached from the signal-proposal executor under its own machine principal beside the acknowledgement it writes. It is a WRITE gated by the proposal that raised it; the read is the current stage, to refuse a move that has already happened",
	"internal/modules/contacts/companynamepromotion.go:CompanyNameCandidates":    "the candidate page the nightly name promoter walks: accounts still named after their domain that a human-typed company name could replace. It runs under the promoter's system actor, and what it answers is an id and the name to be replaced",
	"internal/modules/contacts/companynamepromotion.go:PromoteCompanyNameTx":     "the promotion itself, reading the current name so a corroborated one can replace it and so a name a human already set is left alone. Same system actor, inside the write's own transaction",
	"internal/modules/contacts/renamerecheck.go:recheckCompanyNameForDuplicates": "the duplicate re-check a rename owes: after an account's name changes it may now read like another one, and the review queue must be told. Every caller is a company write — the promoter above, the evidence and canonical-column writers, the merge — and what it produces is a review row inside that write's transaction",
	"internal/modules/contacts/dedupecompany.go:fuzzyCompany":                    "the near-match scan a company write owes its review queue: it reads live accounts to find the one this write reads like. Reached from the create and update paths and from the name promoter, and what it produces is a review row rather than a response",
	"internal/modules/contacts/sitereadactivity.go:siteReadSubjectLabel":         "the subject label stamped on the website-read activity, bounded to a fixed prefix length. Its callers are the read's own lifecycle verbs — begin, defer, finish, retire — running under the site-read worker, and the label goes onto the activity row that names the work, which is read back under the activity grant",
	"internal/modules/capture/autoenrich.go:ListDueCompanies":                    "the enrichment due list: accounts whose domain is ready to be looked up again. It runs on the auto-enrichment sweep's schedule under that job's principal, and what it answers is an id and a domain to ask a provider about",
	"internal/modules/capture/digestcounts.go:readDigestCounts":                  "the per-account counts a capture digest is built from. The digest is assembled by the digest job under its own principal and delivered to the member it is FOR; the account names in it are that member's own captured mail, which is the material the digest exists to summarise",
	"internal/modules/capture/owndomains.go":                                     "the installation's own domains, read so captured mail from a colleague is not filed as an external counterparty. It is workspace configuration reached on the capture path, and what leaves is a set of domain strings rather than any account's record",
	"internal/compose/companyscan/store.go:announce":                             "the scan's subject label, bounded to a fixed prefix length, stamped onto the activity row that announces a website scan. The scan runs under its own bound principal (companyscan/principal.go) and the activity is read back under the activity grant",
	"internal/compose/jobs_finance.go:linkedCustomers":                           "the finance sweep's map from an external customer id to the account it is linked to, so ledger rows can be filed against the right record. It runs under financeSweepPrincipal on the sweep's schedule; the display name is a fallback LABEL for a customer with no link, written to the job's own output",
	"internal/compose/captureofflinedemo.go:accounts":                            "the offline capture demo's fixture accounts, read to seed a mailbox that has something to capture against. It is demo seeding, not a product read path, and it runs where the demo binds its own principal",
	"internal/modules/contacts/companyprofilefieldupsert.go":                     "the profile-field upsert's own liveness guard: `FROM company o WHERE o.id = $1 AND o.archived_at IS NULL` is what makes the INSERT decline against an account archived mid-apply, which its own doc records as the residue a stale write is meant to leave. It is a WRITE, gated by the entry point that composes it, and no company column is selected",
	"internal/modules/contacts/companylogowrite.go":                              "the logo write and the key read beside it. The UPDATE's RETURNING sub-selects read back the slot it just wrote so the caller learns which key now stands, and companyLogoKeyRead answers where one slot's bytes live; both are reached from SetCompanyLogo and the logo serve, past the company object gate",
	"internal/modules/collections/vocab.go":                                      "the collection vocabulary's company arm: which record types a saved view may range over, and the column names each admits. It reads the catalog rather than any account's row — what it produces is the vocabulary a view is validated against",
})

// calleeGatedCompanyReads: a private helper whose every caller asks the object
// gate at the entry point above it.
//
// A verdict of its own rather than folded into lifecycle, for leadreaders'
// reason: the company surface is mostly one store, and calling a plain
// single-row read "lifecycle" would empty that word of meaning in the census
// where it matters most.
//
// What each entry has to establish, and what the reason therefore names: EVERY
// caller of this function asks the company object gate. One that does not makes
// the entry false, and the entry is where a reviewer checks it — this census
// resolves gates FORWARD and cannot resolve them caller-ward, because the call
// graph is by NAME and a gated Store.X would then vouch for an ungated
// Handlers.X. rbacgate_test.go's header records being bitten by exactly that.
var calleeGatedCompanyReads = gatekit.Waive(map[string]string{
	"internal/modules/contacts/company_read.go:readCompany":                       "the shared single-row company read, and the spine every company surface goes through — get, create, update, archive, restore, the merge survivor. Each of those entry points asks auth.Require for the company object and most take a row probe as well; the spine trusts them rather than re-asking inside one transaction. It stamps writability through auth.StampWritable, which is the row half spelled on the way out",
	"internal/modules/contacts/merge_company.go:absorbCompanyReferences":          "the merge's re-parenting arm: every record pointing at the source account is moved to the survivor. Reached only through relinkCompanyAssociations from MergeCompany, past that same object gate and lock, and its effect is an UPDATE rather than a response",
	"internal/modules/contacts/anchorcompany.go:anchorCompany":                    "resolves the installation's own anchor account — the record that represents the company running the installation. Every caller asks the company object gate first: GetAnchorCompany, GetCompanyContext, OnboardingCompanyState, the logo write and the site-read comparison. What it answers is an id",
	"internal/modules/contacts/anchorcompanyread.go:readAnchorCompany":            "the anchor account's own row, behind the same set of gated entry points as anchorCompany above, plus the site-read confirmation which asks the gate at its store method",
	"internal/modules/contacts/anchorguard.go:refuseIfAnchor":                     "the guard that refuses archiving or merging away the installation's own anchor account. Reached from the archive and merge paths, each past auth.Require(company, …) and a row probe, and its only effect is a REFUSAL — it never returns a company column",
	"internal/modules/contacts/company_fact_read.go:ensureCompanyReadable":        "the row probe the fact and profile-field lists take before reading a fact keyed to an account. Its two callers, ListCompanyFacts and ListCompanyProfileFields, each ask auth.Require for the company object at their entry",
	"internal/modules/contacts/companycolumnimages.go:readColdStartColumnImages":  "the cold-start column images an account page shows before enrichment has run. Every caller is a gated company write or read — the deep-read apply, the enrichment apply, the anchor resolve and the cold-start apply beneath them",
	"internal/modules/contacts/companyvisibility.go:refuseStaleCompanyVisibility": "re-reads the visibility column under the row lock so a write built on a stale read cannot re-publish an account its owner has just made private. Reached only from updateCompanyInTx, past auth.Require(company, update) and auth.EnsureWritable, and its only effect is a REFUSAL — the contact column's twin, declared the same way in contactreaders_test.go",
	"internal/modules/contacts/companylogo.go:logoHeldByHuman":                    "the guard that stops an automated logo write overwriting one a human set. Its callers are LogoHeldByHuman and SetCompanyLogo, both past the company object gate, and what it answers is a boolean that only ever KEEPS the existing value",
	"internal/modules/contacts/geocode.go:addressHashInTx":                        "the address hash a geocode result is keyed to, read so a re-geocode of an unchanged address is skipped. Its caller records the geocode past the company gate, and what leaves is a hash",
	"internal/modules/contacts/linkedinreach.go:countUnresolved":                  "the count of a member's LinkedIn connections not yet placed against an account. Its one caller, MyLinkedInReach, asks the company object gate at its entry",
	"internal/modules/contacts/linkedinreach.go:readReachAccounts":                "the accounts a member's network reaches, behind that same gated MyLinkedInReach",
	"internal/modules/contacts/partner.go:listPartnersTx":                         "the partner accounts list, behind ListPartners which asks the object gate at its entry",
	"internal/modules/contacts/mergeface.go:readCompanyFaces":                     "the label and detail line a merge card shows for each side of a pair. Its only caller is DescribeForMerge, which asks auth.Require(entityType, read) — the object name arrives as a PARAMETER, which is why no static reader can see the gate from here — and then narrows the ids through auth.VisibleSubset, so a company the caller cannot see is absent before this runs",
	"internal/compose/company360/graphreads.go:readRelatedCompanies":              "the related-accounts band of the relationship graph, behind the assembly's own gated section reader",
	"internal/compose/company360/suggestionreads.go:readCompanyHeading":           "the account name and lifecycle stage rendered into advice text. Its one caller, UndismissedAdvice, asks auth.ReadGranted(ctx, \"company\") before opening the transaction and answers no advice without it — the object half this census was written to find, added in the same change as this entry",
	"internal/compose/companyrollupread.go:companyReadablePredicate":              "the rollup's readability predicate. CompanyHierarchyRollup asks auth.Require for company, deal and activity in a LOOP over datasource.EntityType constants, so the object arrives as `string(object)` and no static reader can resolve it — the gate is three lines above the transaction this runs in",
	"internal/compose/companyrollupread.go:loadCompanyTree":                       "the recursive account hierarchy the rollup sums over, behind that same three-object gate CompanyHierarchyRollup takes before it opens a transaction",
})

// ruledCompanyReads: a DISCLOSING read the product has ruled needs no company
// grant.
var ruledCompanyReads = gatekit.Waive(map[string]string{})

// notTheCompanyTable: the read pattern matched English prose rather than SQL.
//
// gatekit.TableReadPattern matches `FROM company` anywhere in a string literal,
// and an agent command's human-readable summary — "Remove fact %s from company
// %s" — is a sentence that happens to contain those two words in that order.
//
// Declared rather than fixed in gatekit, for the reason notTheLeadTable gives:
// teaching the shared pattern to tell SQL from prose would cost every other
// census a real read the day a statement is spelled unusually. Over-recognition
// answered by a named declaration is the safe direction.
var notTheCompanyTable = gatekit.Waive(map[string]string{
	"internal/modules/agents/commandsidecar.go:Subject": "`fmt.Sprintf(\"Remove fact %s from company %s\", …)` and its confirm twin — the one-line summary an agent command shows a human in the approval queue. The file holds no SQL at all; every read behind these commands goes through the record seam contacts owns",
})

// deferredCompanyReads: a DISCLOSING read that is still ungated, each naming
// the issue that will close it.
var deferredCompanyReads = gatekit.Waive(map[string]string{
	"internal/modules/deals/offer_render.go:resolveRenderBuyerBlock": "the buyer block of a rendered offer — the account's display name and legal name, on the document a buyer signs. Gated on offer:read and offer:update and never on company. Deferred rather than fixed because the answer is a product ruling and not a patch: an offer with its buyer block withheld is not a smaller document, it is a wrong one, and refusing the render would stop a seat sending an offer they are otherwise entitled to send. Whether offer:update implies seeing the buyer's name is the question. #5586",
	"internal/modules/deals/offer_lifecycle.go:sendSnapshots":        "the same two columns, snapshotted onto the offer at SEND time so the document cannot change under a buyer afterwards. Same gate, same open question, and the pair must be answered together — snapshotting what the render may not show would put the two surfaces into disagreement about one offer. #5586",
})

var companyVerdicts = []namedVerdict{
	{"predicate", predicateCompanyReads},
	{"lifecycle", lifecycleCompanyReads},
	{"callee-gated", calleeGatedCompanyReads},
	{"not-the-table", notTheCompanyTable},
	{"ruled", ruledCompanyReads},
	{"deferred", deferredCompanyReads},
}

// wantMinimumGatedCompanySites is the floor below the count of sites that
// satisfy the gate today. It exists for the reason every floor in this
// directory does: an extractor that stops recognising SQL finds no sites and
// reports nothing, which is indistinguishable from a clean tree.
const wantMinimumGatedCompanySites = 12

var companyReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsCompanyTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsCompanyTable(filePath string, file *ast.File) bool {
	return gatekit.FileReadsTable(filePath, file, companyGate.literal)
}

func TestEveryReaderOfTheCompanyTableCarriesTheObjectGateOrAVerdict(t *testing.T) {
	t.Parallel()
	files := companyReaderScope.Files(t)
	gated := companyGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, site := range companyGate.readSites(parsed, consts.of(t, pkg)) {
			subject := parsed.Path
			if site.function != "" {
				subject += ":" + site.function
			}
			carriesGate := site.holdsGate || callsAGatedHelper(site.calls, gated[pkg])
			if site.function == "" {
				carriesGate = site.holdsGate || companyGate.fileHoldsAGatedFunction(t, parsed, gated[pkg], consts.of(t, pkg))
			}
			verdict := verdictIn(t, subject, companyVerdicts)

			switch {
			case carriesGate && verdict != "":
				t.Errorf("%s carries the company object gate AND a %s verdict — remove the verdict, it "+
					"now describes code that is gated", subject, verdict)
			case carriesGate:
				satisfied++
			case verdict == "":
				t.Errorf("%s reads the company table without the object gate and without a verdict.\n"+
					"  Reading a company discloses who the installation sells to, which is what the "+
					"company grant governs.\n"+
					"  Either ask auth.Require(ctx, \"company\", …) — or auth.ReadGranted where the "+
					"read must degrade rather than refuse — or declare it in "+
					"predicateCompanyReads / lifecycleCompanyReads / calleeGatedCompanyReads / "+
					"ruledCompanyReads with the reason it needs no gate.\n"+
					"  Left alone it is indistinguishable from a read nobody considered.\n"+
					"  The read: %s", subject, site.sql)
			}
		}
	}

	if satisfied < wantMinimumGatedCompanySites {
		t.Errorf("only %d company reads satisfy the object gate, want at least %d — a literal extractor "+
			"that stopped recognising this tree's SQL would report exactly this, and it reads the "+
			"same as a clean tree", satisfied, wantMinimumGatedCompanySites)
	}
	t.Logf("company reads: %d gated, %d predicate, %d lifecycle, %d callee-gated, %d ruled, %d DEFERRED (still disclosing)",
		satisfied, len(predicateCompanyReads.Subjects()), len(lifecycleCompanyReads.Subjects()),
		len(calleeGatedCompanyReads.Subjects()), len(ruledCompanyReads.Subjects()), len(deferredCompanyReads.Subjects()))

	predicateCompanyReads.AssertAllMatched(t)
	lifecycleCompanyReads.AssertAllMatched(t)
	calleeGatedCompanyReads.AssertAllMatched(t)
	notTheCompanyTable.AssertAllMatched(t)
	ruledCompanyReads.AssertAllMatched(t)
	deferredCompanyReads.AssertAllMatched(t)
}
