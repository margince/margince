// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// `contact` is an RBAC object, and until this gate nothing held the object half
// of that anywhere outside the module that owns it.
//
// A contact is a named human being: their name, their title, their address,
// their phone, who they work for and what was said to them. It is the object
// the privacy law in this product's market is written about, and the one whose
// disclosure cannot be undone by an apology.
//
// Two gates cover the compose tier and both say in their own words that they do
// not cover this: rbacgate_test.go judges only exported methods on a module's
// *Store or *Service, and composerowscope_test.go covers ROW scope and refuses
// to count object admission as satisfaction. Between them the object half had
// no fitness function outside internal/modules/contacts.
//
// Fourth and last census over objectGate, after lead, company and deal. It is
// the largest of the four and the one with the most reads that are NOT about a
// contact at all: half of them are capture, enrichment, cohort repair, erasure
// and retention, where the contact is the subject a sweep acts on rather than a
// record anybody is shown. Those are lifecycle, and saying so is most of the
// work — a census that called them disclosures would have buried the four that
// were.

import (
	"go/ast"
	"path"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// contactTable is the table this census is about. Both the pattern and the
// failure messages below read it, so neither can drift from the subject.
const contactTable = "contact"

// contactGate is the subject. No platform spelling gateSeeds it: like the other
// three, a contact has no bespoke admission helper — auth.Require(ctx,
// "contact", …) and auth.ReadGranted(ctx, "contact") ARE the object gate, and
// objectGate reads both off the call.
//
// No row-half allowance either. Four surfaces below reached a contact's name,
// title and email through one shared helper while asking company, or deal, or
// only that the caller was human; the row clause inside that helper is what had
// been standing in for the grant.
var contactGate = objectGate{
	object:              contactTable,
	literal:             gatekit.TableReadPattern(contactTable),
	objectGateSatisfies: true,
}

// predicateContactReads: the contact table appears only inside a JOIN or EXISTS
// that selects or routes records the caller is separately gated on, and nothing
// about the contact reaches the response. The test that puts a read here rather
// than in the gated set: removing the contact condition could only WIDEN the
// result the caller already sees.
//
// The edge reads are the recurring shape. Reading a relationship discloses its
// endpoints AS A PAIR, which relationship.read governs and the endpoints' own
// grants do not cover â so a read projecting r.contact_id with the contact
// joined for liveness and row scope has taken the grant its payload sits behind.
var predicateContactReads = gatekit.Waive(map[string]string{
	"internal/modules/deals/dealunarchive.go":                                         "the relationship restore of a deal un-archive: `contact` appears only in a NOT EXISTS that keeps a link to an archived contact archived. Nothing is selected; removing the condition could only restore more links",
	"internal/modules/contacts/providerboughtfields.go":                               "which of a contact's values a purchase still owns. `contact` appears only inside an EXISTS comparing the title with the ledger's copy, and the statement selects ledger columns (table, field, row id, provider, time). Reached only from readContact, whose callers ask the contact object gate",
	"internal/modules/activities/quietmailbox.go:quietRecordOwner":                    "the owner arm of the cold-queue selector: the contact's owner_id — a USER id, not contact content — decides whether that seat's mailbox is visible enough to call the contact quiet. Nothing of the contact is selected; its only effect is to WITHHOLD a queue entry, never to surface one",
	"internal/compose/capturehealthread.go:captureMailboxHealth":                      "the capture health strip: how many contacts are waiting on a mailbox that has stopped delivering. Counts over the caller's own connector, never a contact column",
	"internal/compose/contact360/nextmeeting.go:nextMeetingSection":                   "the contact's next meeting — `a.id, a.occurred_at, a.subject` — with the contact joined only to reach the meetings that hang off one. Activity columns under the activity grant; removing the join would widen the meetings considered",
	"internal/compose/displaynamerepair.go:selectStaleDisplayNames":                   "`SELECT p.id` — which contacts carry a display name the repair backfill still owes. Ids only, on the backfill's own pass",
	"internal/compose/network/companycoverage.go:countCompanyStakeholders":            "a count of distinct stakeholders at an account, under the edge admission CompanyCoverageFor takes. A number, and deliberately reported as a BOOLEAN incompleteness flag rather than an exact one — the type's own comment says why an exact count would be an oracle",
	"internal/compose/network/companycoverage.go:visibleAccountStakeholders":          "the stakeholder ids and roles behind that same coverage, under auth.EdgeReadScope and the contact ROW scope. Reading an edge discloses its endpoints AS A PAIR, which is what relationship.read governs; no contact column beyond the id the edge already carries is selected",
	"internal/compose/signalextractrule.go":                                           "the visibility roll-up inside the conversation fold the thread extraction reads: a contact's visibility and owner_id decide whether a thread counts as shared or as one owner's. No contact column is selected and the statement produces a thread key",
	"internal/modules/activities/activityrelink.go:repointDisplacedParticipants":      "the relink's repointing arm: which participants must move when an activity's links change. It runs inside the relink write and its effect is an UPDATE",
	"internal/modules/activities/emailparties.go:readEmailParties":                    "the parties on one email, `LEFT JOIN contact` so a party with no contact record still appears. The projection is the activity's own participant rows under the activity grant; the join adds liveness, not a column",
	"internal/modules/activities/followupcomplete.go:completeOpenSystemTasksLinkedBy": "`a.created_at <= (SELECT p.created_at FROM contact p WHERE p.id = $n)` — the cut-off that stops a follow-up task older than the contact being completed by it. A timestamp inside the completing write's own predicate",
	"internal/modules/activities/lasttouch.go:lastTouchEligibility":                   "the contact arm of the cold-queue selector, the twin of the company, deal and lead arms the other censuses file the same way: the contact's liveness and age decide which QUEUE ENTRIES surface, and no contact column reaches the caller",
	"internal/modules/activities/waitingsql.go":                                       "the contact arm of the waiting-thread worklist, off the gated link join. What reaches the caller is the thread their own activity scope already admits",
	"internal/modules/consent/confirmcard.go:confirmCardFor":                          "the card a data subject is shown of THEIR OWN record, behind a confirm link, so they can see what the installation holds before confirming it. The caller is the subject holding a bearer token that proves their mailbox and no CRM grant at all, so a contact grant is not a question that can be asked of them; what bounds the read is that it resolves from the token's own contact",
	"internal/modules/consent/confirmresolve.go:ResolveConfirmToken":                  "resolves a confirm link to the subject it was minted for. Same public edge, same bearer token, and the token is the authority",
	"internal/modules/consent/confirmtoken.go:spendConfirmTokenTx":                    "consumes that token exactly once, so a replayed link cannot submit twice. An UPDATE on the token, with the contact joined for liveness",
	"internal/modules/consent/confirmtoken.go:subjectOfConfirmTokenTx":                "names the subject a spent token acted for, inside the submit's own transaction",
	"internal/modules/consent/gate.go:resolveContact":                                 "resolves an address the CALLER ALREADY NAMED as a send recipient to the one live contact behind it, so a consent grant can be looked up against a subject. What leaves is authorized or not, and why",
	"internal/modules/consent/suppress.go:survivingSubject":                           "`coalesce(merged_into_id, id)` — follows a merged-away contact to the record that survived, so a public stop request lands on the right subject. One id, on the public unsubscribe edge",
	"internal/modules/consent/withdrawalmint.go:bindWithdrawalSubject":                "stamps the subject a one-click unsubscribe link acts for, resolved from the address the send is going TO. Ambiguity leaves the credential unbound rather than stamping one subject's id on another's opt-out link; nothing about the contact reaches the recipient",
	"internal/modules/contacts/company_counts.go:countedEmploymentFrom":               "the employment count on an account card, with the contact joined for liveness and the caller's own row scope. A number, and its object half is asked by the counts assembler above it",
	"internal/modules/contacts/companycommitments.go:countCompanyCommitments":         "the open commitments at an account — a count over activities, with the contact reached only to bound whose commitments they are",
	"internal/modules/contacts/contactvisibility.go:refuseStaleVisibility":            "refuses a visibility change written against a stale version. Its only effect is a refusal",
	"internal/modules/contacts/demote.go:isSharedByOthers":                            "asks whether another record still depends on a promoted contact, so a demote does not archive one something else needs. A boolean that only ever KEEPS a record",
	"internal/modules/contacts/enrichcandidates.go":                                   "the enrichment candidate fragment: which contacts are due a provider lookup. Ids on the enrichment pass",
	"internal/modules/contacts/ensure.go:linkActivityToContact":                       "the link a capture plants between an activity and the contact it resolved, inside the ensure write",
	"internal/modules/contacts/ensurechannel.go:recordChannelDedupeCandidate":         "records a channel-identity near-match for the review queue, inside the ensure write. What it produces is a review row",
	"internal/modules/contacts/ensurechanneladopt.go:adoptEmailRoutedIncumbent":       "adopts an existing contact for a channel identity rather than minting a second, inside that same write",
	"internal/modules/contacts/mergeface.go:readContactFaces":                         "the label and detail line a merge card shows for each side of a pair. Its only caller is DescribeForMerge, which asks auth.Require(entityType, read) — the object arrives as a PARAMETER — and narrows the ids through auth.VisibleSubset first",
	"internal/modules/contacts/observednumbers.go:applyObservedPhone":                 "applies a phone number observed in a signature or on a card, inside the observation write",
	"internal/modules/contacts/observedcontact.go:seedFromColumn":                     "seeds an observed value from the contact's own column so an enrichment does not overwrite what a human typed, inside that same write",
	"internal/modules/contacts/projectstakeholder.go:contactNames":                    "the names shown beside a project's stakeholders, bounded by the contact row scope, inside the stakeholder surface that asks the contact grant at its entry",
	"internal/modules/contacts/relationshipchange.go:visibleContactNames":             "the names an edge-change audit line quotes, under the same row scope and the relationship write's own gate",
	"internal/modules/contacts/sitecontactfields.go:matchSiteContact":                 "matches a contact read off a company website to an existing record, so a site read fills a gap rather than minting a duplicate. Inside the site-read apply",
	"internal/modules/deals/championcover.go:championSeats":                           "the champion seats on a set of deals: `r.deal_id` and an aggregate of `r.contact_id`, with the contact joined for liveness and row scope. Edge endpoints under the edge grant, and no contact column is selected",
	"internal/modules/deals/championcover.go:withheldSeats":                           "the twin that answers whether any seat was withheld from this reader. A boolean per deal, for the reason CompanyCoverage gives about exact counts",
	"internal/modules/deals/engagement.go:Stakeholders":                               "`r.contact_id, r.role` under the contact row scope — the edge's own endpoints, which relationship.read governs. The callers render names through surfaces that ask the contact grant themselves",
	"internal/modules/integrations/backfillselect.go:backlogInTx":                     "`count(*)` of subjects a provider backfill still owes, so the settings screen can show progress. A number about the job",
	"internal/modules/integrations/backfillselect.go:uncoveredSubjects":               "`p.id::text` — which subjects the backfill sweep has yet to cover. Ids consumed by the sweep that runs under its own principal",
	"internal/shared/kernel/retentionscope/retentionscope.go:HeldThroughAnyLink":      "the legal-hold exclusion: it asks whether a linked contact holds the row and, if so, leaves it alone. Its only effect is to keep a row, never to disclose one It is also read by the undo of a project filing and the judgement of a filing about to be made (activities readFilingBlockers): there the arm's boolean becomes the legal_hold or qualifying-deal refusal, so the caller learns only that a hold exists or that another basis keeps the activity — which the refusal has to say to be actionable — and never a field of the record.",
	"internal/modules/privacy/retentionselectors.go":                                  "the retention selectors' contact arm, which only ever narrows what a sweep may delete",
	"internal/platform/auth/signalscope.go:SignalScopeClause":                         "the signal row-scope clause itself, which resolves a signal about a contact to the contact's own visibility. A clause rather than a read: it only ever narrows the signals a caller is shown",
})

// lifecycleContactReads: capture, enrichment, cohort repair, erasure, retention
// and lawful-processing exports. Each is gated by the WRITE it belongs to, or
// runs as PrincipalSystem â which auth.Require short-circuits outright
// (platform/auth/rbac.go), so the gate would admit them anyway and the entry
// records why asking was never the point.
var lifecycleContactReads = gatekit.Waive(map[string]string{
	"internal/modules/privacy/reportingretention.go:reportingRetentionSelector": "Retention lifecycle checks legal holds across all linked subjects before selecting an edition for expiry; it returns edition ids to the audited eraser, not source records to a reader.",
	// Deal Scout: its system-principal evidence read and the suggestion writer's own conditions.
	"internal/modules/activities/meetingcounterparty.go:HeldMeetingCounterparties": "the held-meeting walk Deal Scout reads as the system principal: a participant's contact is joined only for liveness on the way to their employer, and no contact column is selected — what leaves is an (activity, company) pair",

	"internal/modules/capture/replyevidence.go:AnsweredContactsTx":                      "the verdict worker's reconciling pass, under the system principal: ids, owners and addresses of owner-scoped contacts whose owner has been answered, handed straight to the promotion that re-checks them. Work to do rather than a contact anyone is shown",
	"internal/compose/noticecaseopen.go:openFor":                                        "the notice consumer's own pass under the system principal: it locks the contact and reads only whether it is archived, so a withdrawn record is not given a duty. Nothing about the contact leaves",
	"internal/compose/captureofflinedemo.go:fillParties":                                "the offline capture demo's fixture parties, read to seed a mailbox that has something to capture against. Demo seeding rather than a product read path",
	"internal/modules/capture/digestcounts.go:readDigestCounts":                         "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/noisemailscope.go":                                        "the noise-mail scope fragment: which captured mail counts as noise, reaching the contact for liveness. A clause on the capture pass, never a projection",
	"internal/modules/capture/purgeselect.go:SelectPurgeableContactsTx":                 "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/senderslist.go:SendersFor":                                "the sender decisions a member is shown for their OWN mailbox, under auth.RequireHuman bound to that member's user id. The contact table appears only in an EXISTS answering whether a record already exists for an address the caller supplied — and that EXISTS is visibility-scoped on purpose, its own comment recording that an unscoped one would answer \"does your colleague know this contact\" for any address a seat cared to guess",
	"internal/modules/capture/sinkensure.go:priorDispositionTx":                         "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/sinkmaillinks.go:mailParticipantsWithContacts":            "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/sinkmeetinglinks.go:meetingParticipantsWithContacts":      "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/sinkpersonal.go:ContactsOrphanedByPrivacyTx":              "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/sinkreply.go:channelReplyContact":                         "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/sinkreply.go:mailReplyContact":                            "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/strandedcontacts.go:NoiseJudgedContacts":                  "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/capture/sinkpersonal.go:SettledPersonalThreads":                   "the link-reconcile sweep's own selector, run under the system principal with no request and no human actor — the same posture as NoiseJudgedContacts beside it, and a gate here would refuse its only caller. It writes nothing, and it discloses nothing about a contact: the contact table is reached only inside an EXISTS, so no contact column leaves the query. What it ANSWERS is a thread key and the seat's own user id — which private conversations still have work owing, never who is on them. The write that follows is confined by ContactsOrphanedByPrivacyTx re-read on the retraction's own transaction, and by contacts.RetractCaptureOnlyContactTx re-checking the full eligibility under the contact row's lock",
	"internal/modules/capture/strandedcontacts.go:StrandedContacts":                     "the capture lane's own pass. It runs under the capture worker's principal — or, for the surfaces a member reaches, under auth.RequireHuman bound to that member's OWN mailbox — and what it answers is work to do rather than a contact anyone was shown",
	"internal/modules/contacts/cohortpromote.go:ContactsOwedACohortRepair":              "which contacts the link-reconcile sweep still owes a cohort repair. It runs from that sweep under its system context and answers work to do",
	"internal/modules/contacts/cohortpromote.go:DomainsOwedTheirContacts":               "the domain backlog behind the same sweep: accounts whose domain has contacts carrying no employment edge yet",
	"internal/modules/contacts/cohortpromote.go:PromoteContactCohortTx":                 "the repair itself, inside the sweep's transaction",
	"internal/modules/contacts/companynamepromotion.go:CompanyNameCandidates":           "the candidate page the nightly name promoter walks, under the promoter's system actor",
	"internal/modules/contacts/companynamepromotion.go:loadSignatureCompanyNames":       "the signature-derived names that pass corroborates against, on the same schedule and the same actor",
	"internal/modules/contacts/contact_children.go:readContact":                         "the shared single-row contact read behind the merge-state resolution. Its callers ask the contact object gate at their entry points",
	"internal/modules/contacts/contactattachlock.go:lockContactForAttach":               "takes the contact's row lock before an employment edge is attached, so two importers cannot plant conflicting edges. Inside each attaching write",
	"internal/modules/contacts/contactprivate.go:CaptureOnlyHoldersOfAddressTx":         "which capture-only contacts hold an address, so a privacy retraction knows what to withdraw. Reached from the confidentiality retraction pass under its own principal",
	"internal/modules/contacts/contactprivate.go:RetractCaptureOnlyContactTx":           "the retraction itself, in that same pass",
	"internal/modules/contacts/creatededupe.go:recordIfReview":                          "the near-match scan a contact write owes its review queue. Its callers are writes, and what it produces is a review row inside the caller's transaction",
	"internal/modules/contacts/ensurerecords.go:fileReviewPair":                         "the same review row on the CAPTURE path, inside the ensure's own transaction and reached only after that ensure has created the contact. The one column it reads is the incumbent's full_name, and it is read to be WRITTEN into the pair's evidence as the detection-time snapshot a reviewer compares against — never returned to a caller and never rendered to whoever's mail triggered the capture. The queue that later shows it carries its own gate",
	"internal/modules/contacts/dedupe.go:fuzzyContact":                                  "the fuzzy arm of that same scan, behind Resolve on the capture path",
	"internal/modules/contacts/displaynamerefresh.go:RefreshDisplayNameTx":              "recomputes a contact's display name after its parts change, inside the write that changed them",
	"internal/modules/contacts/domaintriage.go:OnDomain":                                "the contacts a domain triage decision applies to, inside the resolving write",
	"internal/modules/contacts/domaintriageresolve.go:domainEmploymentCandidates":       "which contacts a resolved domain owes an employment edge, inside that same write and the backlog sweep",
	"internal/modules/contacts/domaintriageresolve.go:plantDomainEmployment":            "plants those edges, in the same transaction",
	"internal/modules/contacts/employment_import_batch.go:employmentImportContacts":     "the batch an employment import is applying, inside the import's own write and its sweep",
	"internal/modules/contacts/enrichment.go:DuplicateCluster":                          "the duplicate cluster an enrichment hold is about, on the enrichment path",
	"internal/modules/contacts/enrichment.go:EnrichmentFence":                           "the fence that stops two enrichments running on one subject, inside the holding write",
	"internal/modules/contacts/enrichment.go:SubjectNameOnly":                           "the subject's name for an enrichment request, on that same path",
	"internal/modules/contacts/ensurenamefill.go:completeContactName":                   "fills a contact's missing name from a captured participant, inside the capture write",
	"internal/modules/contacts/ensurenamefill.go:displayNameSetByHumanTx":               "the guard that stops that fill overwriting a name a human typed. Its only effect is to KEEP the existing value",
	"internal/modules/contacts/meetingcohort.go:linkAttendedMeetings":                   "links the meetings a repaired cohort attended, inside the repair's transaction",
	"internal/modules/contacts/participantname.go:FillParticipantNamesTx":               "fills participant names across a captured thread, on the capture path",
	"internal/modules/privacy/erasure.go:anonymizeSubjectRows":                          "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/erasure.go:refuseContactUnderLegalHold":                   "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/erasure_channels.go:subjectDisplayName":                   "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/erasure_leadtwins.go:anonymizeLeadTwins":                  "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/erasure_provider.go:subjectContactIDs":                    "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/erasure_rivals.go:anotherLiveContactHoldsAChannelAccount": "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/erasure_rivals.go:anotherLiveContactHoldsAnEmail":         "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/retentionactions.go:anonymizeContactRecord":               "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/sar.go:appendSubjectCustomValues":                         "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/sar.go:subjectIdentities":                                 "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/sar.go:subjectReach":                                      "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/sarsections.go:sarIdentitySections":                       "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/privacy/sarsections.go:sarRecordSections":                         "runs as the system principal on a request or sweep a human already authorised, and the rows it reaches are bounded to the subject the erasure, retention rule or export is about. Respecting a caller's grants here would under-delete or under-export, which is the defect the pass exists to prevent",
	"internal/modules/search/embedgen.go":                                               "the per-entity source text the embedding lane indexes, mirroring the contact's search_tsv columns so the vector and lexical lanes agree about one text. It runs on the indexing path under the system principal; the SEARCH that later reads the index row carries the caller's own scope",
})

// calleeGatedContactReads: a helper whose every caller asks the contact object
// gate at the entry point above it.
//
// What each entry has to establish: EVERY caller of this function asks the
// contact object gate. One that does not makes the entry false, and the entry is
// where a reviewer checks it â this census resolves gates FORWARD and cannot
// resolve them caller-ward, because the call graph is by NAME.
var calleeGatedContactReads = gatekit.Waive(map[string]string{
	"internal/modules/contacts/unarchive.go":                                            "the un-archive shapes: the contact's label and state under FOR UPDATE, and contact liveness inside a relationship restore. Read only through RestoreContactTx and RestoreCompanyTx, which ask auth.Require for the delete grant and auth.EnsureWritable on the row first (ensureRestorable); the label goes back only to the caller who may restore the record",
	"internal/modules/privacy/suppressionreapply.go:contactsMatchingSuppression":        "every live address in the installation, hashed against the erasure-suppression list to find subjects a restore or a re-import brought back, and never returned — the ids go straight into EraseContact. Its one caller is ReapplySuppressions, which asks auth.Require(ctx, \"contact\", delete) before reaching it, and EraseContact asks the same gate again per subject under its own lock. It is deliberately installation-wide and unscoped by row: the suppression list has no tenant predicate and nothing for one to key on (storekit/suppression.go on ADR-0091 and ADR-0061), and a pass that only looked at the rows one seat may see would leave a resurrected subject standing wherever that seat cannot reach — which is most of the installation, and exactly where nobody would notice",
	"internal/compose/magic/undoversion.go":                                             "the version an undo on the magic receipt is sent with, for the restore route's If-Match; no other column. Reached only from magic.Service.Read and LineRecords, and only for a record their done lane already placed through doneForType, which asks auth.Require for the object grant and renders auth.ScopeClauseFor on the same row; the restore write re-asks everything under its own lock",
	"internal/modules/contacts/contactrenamerecheck.go:recheckContactNameForDuplicates": "the renamed contact's own name and addresses, read to score it against the rest of the workspace for the duplicate queue and never returned. Reached only from UpdateContact through recheckIfRenamed; UpdateContact asks auth.Require(ctx, \"contact\", update) and auth.EnsureChangeable on the row first",
	"internal/modules/contacts/contactvisibility.go:patchNarrowingByHand":               "the prior narrowing reason, read only to write its audit before-image. Reached from UpdateContact through guardVisibilityWrite; UpdateContact asks auth.Require(ctx, \"contact\", update) and auth.EnsureWritable on the row first",
	"internal/modules/contacts/narrowing.go:recordNarrowingTx":                          "the prior narrowing reason of the row an ensure just resolved, read to decide whether a decision may overwrite it and never returned. Its one caller is ensureContact, which asks auth.Require(ctx, \"contact\", create) first",
	"internal/compose/contact360/commercial.go:committeeFor":                            "the buying committee on a contact page: names, roles and photos of the other seats on the deal. commercialSection asks requireRead(ctx, \"contact\") — the package-local wrapper over auth.Require — before reaching it",
	"internal/compose/introseams.go:accountContacts":                                    "the contacts offered as an introduction route. Its caller asks the contact object gate before composing the seam",
	"internal/compose/meetingbrief/meeting.go":                                          "the pre-meeting brief's room statement. scopeFor asks auth.ReadGranted for each object it scopes, so a caller with no contact grant gets that join matched away; the census cannot see it because the object arrives as a PARAMETER",
	"internal/compose/network/contactgraphcompany.go:readCompanyContacts":               "the colleagues band of the contact graph, behind the graph assembly's own contact gate",
	"internal/modules/contacts/linkedinmatch.go:emailMatchCandidates":                   "the address arm of the LinkedIn match. Its one caller chain is matchInTx ← runLinkedInMatch, which asks auth.Require(ctx, \"contact\", read) before either tier runs; the row scope and the per-contact write clause travel inside the SELECT itself",
	"internal/modules/search/graphactivitysubjects.go:employerSubjects":                 "the employer band beside it, which walks contacts to reach the companies they work for and asks auth.ReadGranted for the company half itself",
	"internal/modules/search/graphactivitysubjects.go:participantSubjects":              "the participant band of a record's context — contact ids and names. Its enclosing read asks the contact object gate before assembling the bands",
})

// ruledContactReads: a DISCLOSING read the product has ruled needs no contact grant.
var ruledContactReads = gatekit.Waive(map[string]string{
	"internal/modules/activities/projectfiling.go": "the undo of a project filing locks the linked record FOR SHARE so a legal hold placed on it cannot land between the undo reading the holds and clearing the class. It takes a shared row lock (shareLockQualifyingRecords) on the records the activity is linked to, so the undo of a project filing serializes with a hold or a stamp being written; it selects only a row count, no field, and returns nothing to the caller.",
	"internal/modules/privacy/legalholdlist.go":    "the litigation-hold census: one UNION over the five holdable tables, selecting an id and a display name for rows where legal_hold is set. Gated on the retention-policy authority and refused to a non-human principal at its own entry, which is the posture the sibling restricted-records list already takes — a controller asked what a hold is preserving has to be told which records those are, and the grant that governs the retention ladder is the one that governs seeing what overrides it. No field of the record is read beyond its name",
})

// deferredContactReads: a DISCLOSING read that is still ungated, each naming
// the issue that will close it.
var deferredContactReads = gatekit.Waive(map[string]string{})

// notTheContactTable: the read pattern matched something that is not the table.
// Empty for : unlike , the word is not a SQL keyword, and the
// pattern's delimiter set already keeps contact_room and contact_stage_history out.
// notTheContactTable: the read pattern matched something that is not this table.
//
// Empty for `contact`, and the emptiness is a finding rather than an omission:
// the word is not also a SQL keyword, and TableReadPattern's delimiter set keeps
// `contact_email`, `contact_channel_identity` and the rest of the satellite
// tables out, because an underscore is not a delimiter.
var notTheContactTable = gatekit.Waive(map[string]string{})

var contactVerdicts = []namedVerdict{
	{"predicate", predicateContactReads},
	{"lifecycle", lifecycleContactReads},
	{"callee-gated", calleeGatedContactReads},
	{"not-the-table", notTheContactTable},
	{"ruled", ruledContactReads},
	{"deferred", deferredContactReads},
}

// wantMinimumGatedContactSites is the floor below the count of sites that
// satisfy the gate today. It exists for the reason every floor in this
// directory does: an extractor that stops recognising SQL finds no sites and
// reports nothing, which is indistinguishable from a clean tree.
const wantMinimumGatedContactSites = 22

var contactReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsContactTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsContactTable(filePath string, file *ast.File) bool {
	return gatekit.FileReadsTable(filePath, file, contactGate.literal)
}

func TestEveryReaderOfTheContactTableCarriesTheObjectGateOrAVerdict(t *testing.T) {
	t.Parallel()
	files := contactReaderScope.Files(t)
	gated := contactGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, site := range contactGate.readSites(parsed, consts.of(t, pkg)) {
			subject := parsed.Path
			if site.function != "" {
				subject += ":" + site.function
			}
			carriesGate := site.holdsGate || callsAGatedHelper(site.calls, gated[pkg])
			if site.function == "" {
				carriesGate = site.holdsGate || contactGate.fileHoldsAGatedFunction(t, parsed, gated[pkg], consts.of(t, pkg))
			}
			verdict := verdictIn(t, subject, contactVerdicts)

			switch {
			case carriesGate && verdict != "":
				t.Errorf("%s carries the contact object gate AND a %s verdict — remove the verdict, it "+
					"now describes code that is gated", subject, verdict)
			case carriesGate:
				satisfied++
			case verdict == "":
				t.Errorf("%s reads the contact table without the object gate and without a verdict.\n"+
					"  Reading a contact discloses a named human being — their name, title, "+
					"address and employer — which is what the contact grant governs.\n"+
					"  Either ask auth.Require(ctx, \"contact\", …) — or auth.ReadGranted where the "+
					"read must degrade rather than refuse — or declare it in "+
					"predicateContactReads / lifecycleContactReads / calleeGatedContactReads / "+
					"ruledContactReads with the reason it needs no gate.\n"+
					"  Left alone it is indistinguishable from a read nobody considered.\n"+
					"  The read: %s", subject, site.sql)
			}
		}
	}

	if satisfied < wantMinimumGatedContactSites {
		t.Errorf("only %d contact reads satisfy the object gate, want at least %d — a literal extractor "+
			"that stopped recognising this tree's SQL would report exactly this, and it reads the "+
			"same as a clean tree", satisfied, wantMinimumGatedContactSites)
	}
	t.Logf("contact reads: %d gated, %d predicate, %d lifecycle, %d callee-gated, %d ruled, %d DEFERRED (still disclosing)",
		satisfied, len(predicateContactReads.Subjects()), len(lifecycleContactReads.Subjects()),
		len(calleeGatedContactReads.Subjects()), len(ruledContactReads.Subjects()), len(deferredContactReads.Subjects()))

	predicateContactReads.AssertAllMatched(t)
	lifecycleContactReads.AssertAllMatched(t)
	calleeGatedContactReads.AssertAllMatched(t)
	notTheContactTable.AssertAllMatched(t)
	ruledContactReads.AssertAllMatched(t)
	deferredContactReads.AssertAllMatched(t)
}
