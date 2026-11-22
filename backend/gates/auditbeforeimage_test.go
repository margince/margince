// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// An audited update says what it changed from.
//
// audit_log carries before and after images. A row whose action is `update` and
// whose before is absent records that something changed and cannot say what it
// changed from — so field history renders half a change, and nothing can restore
// the value. There is no other place that value is written down: an audit row
// cannot be written after the fact.
//
// Two kinds of write reach the same column, and until AuditEvent existed they
// were spelled identically. One has no prior field state at all — a secret
// rotated, a delivery replayed, a tag applied — and its absent before-image is
// the honest answer. The other simply did not look. This gate is what keeps them
// apart: the first says so by the door it calls, and says why in a waiver here.
//
// It is the census half of a pair. storekit's refusal is the chokepoint, and it
// is the authority. The sites come from the audit census (auditdoorsites_test.go):
// every call into a storekit door, with its verb read by the type checker. A
// verb built at runtime stays unread (privacy/retentionactions.go passes one
// read from a database row). A before-image read only at the door: a wrapper
// that forwards it as a parameter is left to the refusal. The census also
// reports any other writer of audit_log, including a direct INSERT. The
// integration harness's tombstone seed in harnessseed.go carries the
// integration build tag, so the census never reads it. Between them, the gate
// says who writes the table and the chokepoint says what a door will accept.
//
// Only `update` is judged. `archive` sites also pass an absent before-image and
// are deliberately left alone: un-archiving is not a before-image replay but
// `SET archived_at = NULL` plus whatever that record's archive took down with
// it, which is a per-type product decision and not this gate's to force.

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// eventShapedUpdates: writes that record an occurrence on a record rather than a
// change to its fields, keyed by `path:function`. Each entry states what the
// write has INSTEAD of a prior state — a reason that only restates the subject
// is refused by gatekit.
//
// The FUNCTION is the unit of ratification, not the call. A function that audits
// twice — contacts/linkedinmatchapply.go does — is judged once, so ratifying it
// says every audited update it makes is occurrence-shaped. Splitting the two
// would need a line number in the key, and a key that moves whenever the file
// above it moves is a waiver that goes stale for no reason.
//
// Empty is a result, not a default: it would mean every audited update in the
// tree records what it changed from.
var eventShapedUpdates = gatekit.Waive(map[string]string{
	"internal/modules/consent/restrictionlift.go:queueRestrictionLiftTx":             "an admin asking to lift a restriction of processing. An OCCURRENCE: no field of the contact changes and the stop stays as it was until the notice is sent. What it has instead of a prior state is the stop it names and the admin's reason",
	"internal/modules/privacy/linkedhistoricalaudit.go:correctLinkedHistoricalEntry": "certifies removal of an old audit row payload. A before-image would retain the personal values being removed; the original row metadata and a separate proof identify the change",
	"internal/modules/privacy/historicalaudit.go:redactHistoricalRecord":             "a payload redaction of an earlier audit entry, certified by its immutable record ID. A before-image would keep the personal values this correction removes. The original audit metadata stays fixed and a redacted marker replaces its payload",
	"internal/modules/contacts/company_offered_value.go:RecordOfferedValues":         "a feed's offer is kept beside the company's own fields and changes none of them, so there is no prior field state to image. The audit row names the source and the fields offered, never the values, which may hold personal data.",
	"internal/modules/capture/meetingguests.go:SettleMeetingRosterTx":                "a calendar's roster was settled on a meeting already on file: participant rows it lists were ADDED, rows it no longer lists were retired, and contact links were added. No field of the activity changed. The rows are a set keyed by the calendar that wrote them, not a field with a prior value, and an image of their addresses would outlive an erasure. What it has instead is the count of guests added and retired and of links added, plus the repair run as evidence when the data task wrote them",
	"internal/modules/capture/meetingdocuments.go:announceTx":                        "a later calendar sync added, archived or restored the notes or transcript document links on a meeting already on file. No field of the activity changed; each link row has its own create, archive or restore audit. What it has instead is the count of documents changed",
	"internal/modules/activities/meetingcopyretire.go:carryMeetingDocumentLinksTx":   "a meeting copy retiring onto the meeting that stays handed its notes-document link rows to it. No field of the surviving activity changed; each moved link row has its own update audit with the meeting it left as the before-image. What it has instead is the count of documents handed on, with the retired copy as evidence",
	"internal/modules/activities/meetingcopysplit.go:UndoMeetingCopyGuestsTx":        "a repair split a wrong meeting-copy fold and took back the guest rows the fold had carried onto the survivor. No field of the activity changed; the rows are a set the fold added, and an image of their addresses would outlive an erasure. What it has instead is the count of guests taken back, with the split-off copy as evidence",
	"internal/modules/activities/requesttaskrestore.go:recordRequestAcceptance":      "accepting an already-live automatic reminder records a human decision, not a field change. The accepted_request evidence is the durable admission fact; archived restoration separately records its real before/after field values",
	"internal/modules/privacy/commitmenterasure.go:eraseCommitmentTask":              "held evidence keeps its activity fields unchanged. The occurrence invalidates derived indexes and announces that erased source evidence withholds the task; the audit records that cause rather than a fabricated field update",
	"internal/modules/activities/projectfilingundo.go:recordProjectFilingUndone":     "a member took an activity back out of a project and withdrew the retention class that filing gave it. An OCCURRENCE and not a field edit: retention_class is not a field a reversal may put back, and a before-image of it would invite one. What the row has instead of a prior state is stronger — the evidence names the class that was withdrawn, every project filing that qualified it (by the name frozen when it did), the deciding member's name and the reason they wrote, which together answer the question a supervisory authority asks of a shortened retention",
	"internal/modules/consent/satellitecarry.go:auditSatelliteCarry":                 "a merge moved the retired record's consent links and bases onto the survivor, so rows were ADDED to a record that did not hold them. No field of the survivor changed, and there is no prior state to record. What it has instead is the count moved, and evidence naming every moved row by table and the record it came from — so an auditor asking why an old unsubscribe link now withdraws this contact reaches the merge that brought it",
	"internal/modules/consent/stopcarry.go:CarryStopsTx":                             "a merge retired one record and the stop it held now also applies to the survivor, so a stop was ADDED to a record that had none. There is no prior state to record because the survivor had no such row: this is not a field of theirs changing, it is a fact about the merge landing on them. What it has instead is the count of stops carried, and the row itself names its origin in carried_from — so an auditor asking why this contact is suppressed reaches the record that actually objected, and the merge that brought it here, without needing a before-image of a row that did not exist",
	"internal/modules/consent/overridecarry.go:auditAndEmitCarriedOverrides":         "the same merge landing an override rather than a stop on the survivor, split into its own function from CarryOverridesTx so the read query and the write stay legible on their own. The same OCCURRENCE shape CarryStopsTx has and for the same reason: there is no prior state to record because the survivor had no such row, and what it has instead is the count of overrides carried plus one event per carried row on a contact survivor — the row itself names its origin in carried_from, so an auditor asking why a send to this contact was overruled reaches the rep who vouched and the merge that brought it here, without needing a before-image of a row that did not exist",
	"internal/modules/consent/lift.go:revokeStopTx":                                  "somebody with the authority to do so took back a stop, so mail may resume. An OCCURRENCE and not a replacement: no field of the contact changes, and the row it revokes is not edited — revoked_at is set and the row stands, because a stop that was lifted is still a thing that happened. What it has instead of a prior state is the level the stop was recorded at and the level that lifted it, which together say the comparison that permitted this and let an auditor check it without re-deriving it from a row that no longer says what it was",
	"internal/modules/consent/bouncesuppress.go:RecordHardBounceTx":                  "the receiving mail system said an address is permanently gone, so this installation stops writing to it. An OCCURRENCE and not a replacement: no field of anybody's record changes, the row is created and never edited, and a lift revokes it rather than rewriting it. The entity is named at runtime because a bounce is a fact about an ADDRESS and the address may belong to a contact, to a lead, or to nobody at all — so the audit hangs from the contact when one owns the address and from the delivery that died when none does, which is the only other identity this fact has. What it carries instead of a prior state is the kind of stop and the level it was decided at, which is what an auditor asking why mail to this address is refused needs. The address itself is deliberately NOT in the payload: audit_log is append-only, so an address written there would outlive the Art. 17 erase that clears the row",
	"internal/modules/consent/bouncesuppress.go:LiftAddressStop":                     "an address that refused delivery works again — usually because somebody corrected a typo — so the stop the machinery wrote is taken back. The same OCCURRENCE shape revokeStopTx has and for the same reason: revoked_at is set and the row stands, because a stop that was lifted is still a thing that happened. The entity is the SUPPRESSION ROW rather than a contact, because a bounce stop deliberately names no contact — a contact-scoped row would refuse every address that record has — so the row is the one identity this fact reliably carries. What it has instead of a prior state is the kind that was lifted and the reason the caller gave, which together answer who decided this address works again and why",
	"internal/modules/consent/withdrawalpress.go:StopForCredentialTx":                "a lead or an address-only recipient pressed the unsubscribe link in their mail, and holding no per-purpose consent rows there is nothing to withdraw — so the press records a stop. The same OCCURRENCE shape suppressAdmittedTx has, for the same reason: no field of the record changes, the row is created and never edited, and a lift revokes it rather than rewriting it. The entity is named at runtime because the subject is genuinely one of two — the lead when a lead holds the address, and nothing at all when no record does, in which case the row is its own record and no audit entry is written. What it carries instead of a prior state is the kind of stop and where it came from, which is what an auditor asking why this lead is suppressed needs",
	"internal/modules/recordlinks/write.go:recordLinkChange":                         "a user linking two records, or removing a link. An OCCURRENCE and not a replacement: no field of either record changes. What moved is a row in record_link, which holds the type and both ends, and the audit row names the link by id and type. The subject is the record the link starts on, an entity named at runtime, and the other end stays off the image because a reader of one record may not be allowed to see the other",

	"internal/modules/consent/suppress.go:suppressAdmittedTx":                "somebody recorded that a subject asked us to stop writing to them. An OCCURRENCE and not a replacement: no field of the contact changes, and there is no prior state to image because a stop either was on file or was not — the row is created, never edited, and lifting one revokes it rather than rewriting it. What it has instead of a prior state is the kind of stop and the authority it was recorded at, which together say what was asked and who may lift it. The subject's own words for the reason stay on the row and out of the audit payload, because an audit entry is read by contacts the reason was not given to",
	"internal/modules/consent/overridechain.go:auditAndEmitRevokedOverrides": "somebody with the authority to do so took back a standing override, so the machine-level refusal it vouched against applies again. The same OCCURRENCE shape revokeStopTx has and for the same reason: no field of the contact changes, and the override it revokes is not edited — revoked_at is set and the row stands, because an override that was revoked is still a thing that happened. What it has instead of a prior state is the level the override was recorded at and the level that revoked it, which together say the comparison that permitted this and let an auditor check it without re-deriving it from a row that no longer says what it was",
	"internal/modules/consent/authorizebasis.go:recordBasis":                 "the lawful ground an outbound message was permitted on, written down when the engine concludes it. An OCCURRENCE and not a replacement: no field of the contact changes — the contact is the subject because a reader asking why we wrote to somebody looks the contact up, not a basis id — and there is no prior state to image, because a ground either was on file or was not: the row is created, never edited, and the check beside it refuses a second one while the first is live. What it has instead of a prior state is the basis kind and the category the engine resolved, which together say what was concluded and on what footing. The row itself carries the conversation, the window and the source message",
	"internal/modules/contacts/sdrhandoff.go:ResubmitHandoff":                "a recycled handoff reopened. An OCCURRENCE and not a replacement, on the CAS: the UPDATE carries `status = recycled`, so a write that lands is one where the prior state WAS recycled — an image would record that one word for every row, forever, which the verb already says. What it has instead of a prior state is sdr_handoff_event, written in the same transaction and append-only, carrying every round this prospect has been through",
	"internal/modules/assurance/bundle.go:CloseCycle":                        "a pass of assurance ending. An OCCURRENCE and not a replacement, and the CAS makes that exact: the UPDATE carries `closed_at IS NULL`, so a write that lands is one where the cycle WAS open — an image would record that one fact for every row, forever, which the verb already says. A second close reports ErrNotFound and the first moment stands. What the audit row has instead of a prior state is WHO decided the window was over, which the closed_at timestamp on the row cannot say",
	"internal/modules/contacts/sdrhandoff.go:DecideHandoff":                  "an account executive answering a handoff, once. The CAS is what makes the occurrence exact rather than a claim: the UPDATE carries `status = submitted`, so a write that lands is one where there WAS no prior decision — an image would record `submitted` for every row, forever, which is the state the id already implies. A second decision reports itself already-decided and the first stands. What this has instead of a prior state is stronger than one: sdr_handoff_event, written in the same transaction and append-only, carries every transition including the submission this answers, so how the handoff reached its state is recoverable in full rather than one step back",
	"internal/modules/activities/replyverdict.go:setReplyVerdictTx":          "a classifier reading one inbound message and saying whether it was a positive answer. An OCCURRENCE and not a replacement, and the CAS is what makes that exact rather than a claim: the UPDATE carries `reply_verdict IS NULL`, so a write that lands is one where there WAS no prior verdict, and an image would record the absence of one for every row forever. A second judgement reports itself unapplied and the first stands. What the audit row has instead of a prior state is the verdict, the classifier that reached it, and the principal that wrote it",
	"internal/modules/activities/replyverdict.go:CorrectReplyVerdict":        "a human overruling that reading — and this one DOES replace, so the answer is not the CAS argument above. What it has instead of a before-image is stronger than one: activity_reply_verdict_history, written in this same transaction and append-only, already carries every prior verdict with who decided it and when. A before-image on the audit row would duplicate the immediately preceding history row, and duplicating it is worse than omitting it — two records of one prior state can disagree after a later migration touches one, and a reader has no way to know which is the account. The audit row answers who overruled the classifier; the history answers what it said before",

	"internal/modules/capture/widenhistory.go:ShareHistory": "a seat re-opening the mail their own counterparty hold caught. An OCCURRENCE and not a replacement: NO field of the seat changes — the seat is the subject because the act is theirs and is what a reader investigating who published a lawyer's correspondence would look up. There is no prior state to name either, because the thing that changed is a SET of import rows, each moving from its own posture, and an image listing them would name the very correspondents this feature keeps private. What it has instead is the count of what moved, plus one activity.updated per re-opened message carrying its new audience",

	"internal/modules/capture/strandedcontacts.go:AskWhoseRecord": "a question being raised about a contact the ceiling refused to ask about at capture. An OCCURRENCE and not a replacement: NO field of the contact changes — the record is exactly as the capture left it, and what moved is a row in the ledger saying somebody will now be asked whose record this is. The contact is the subject because the question is about them, and because a reader wondering why a private contact became visible to the workspace looks the contact up, not the ledger. There is no prior state to name either: the whole guard is that no row for this address existed, so the image would be the absence of one. What it has instead is the address the question was raised on, and the moment it was raised",

	"internal/modules/contacts/cohortpromote.go:promoteCohortTx": "mail the workspace already held being filed under the record it belongs to. An OCCURRENCE and not a replacement: no field of the CONTACT changed — the row is the subject because it is what the repair is about and what a reader would look up — and the messages that moved held no prior owner for an image to name, because the whole guard is that they were linked to nobody. What it has instead of a prior state is the count of what moved, plus one activity.updated per message naming the record it is now filed under",

	"internal/modules/activities/worklistpin.go:recordPin":   "a rep putting one row at the top of their own day. An OCCURRENCE and not a replacement: NO field of the row changes — it is exactly the row the lanes assembled — and what moved is a line in a side table saying one reader wants it first. The row's own record is the subject because that is what the pin is about. There is no prior state to image either: a pin either stood or did not, and the insert does nothing when it already did, so an image would record the absence of one for every row that has never been pinned. What it has instead is the action, the lane the row came from, the row's own id and whose day was reordered — the id on the IMAGE rather than only as the subject, because a folded group's identity is synthetic and cannot be an entity id at all",
	"internal/modules/activities/worklistdone.go:recordDone": "a rep taking one row off their own Worklist. An OCCURRENCE, as a pin is: no field of the row changes, and what moved is a line in a side table saying one reader is done with it. There is no prior state to image: the Done either stood or did not. What it has instead is the action (done or reopened), the lane the row came from, the row's own id and whose list it cleared. The id is on the image because a folded group's identity is synthetic and cannot be an entity id",

	"internal/modules/activities/linksuggestion.go:recordLinkSuggestion": "an extension proposing a record for a message, and a reader deciding it. An OCCURRENCE and not a replacement: no field of the activity changes. What moved is a row in activity_link_suggestion, which holds the target, the unit, who decided and when, and the audit row points at it. An accept also files the message through the relink, which writes its own audit row with its own before image",
	"internal/modules/activities/meetingreadiness.go:recordReadiness":    "an attendee saying they are ready for a meeting, or taking it back. An OCCURRENCE and not a replacement: NO field of the meeting changes, and what moved is a row in meeting_readiness that holds for that attendee alone. There is no prior field state to image. What it has instead is the new state, the preparation revision it was given at as evidence, and the actor or the user an agent acted for, which names the attendee",
	"internal/modules/activities/disposition.go:recordDisposition":       "a rep deciding what to do about a waiting message. An OCCURRENCE and not a replacement: NO field of the activity changes — the message is exactly as the customer sent it, and what moved is a row in a side table saying somebody put it down. The activity is the subject because it is what the judgement is about and what a reader investigating would look up. What it has instead of a prior state is the decision itself plus the moment a snooze lifts, and the side tables carry set_by and set_at, so who judged and when is recoverable from the rows the audit row points at",

	"internal/modules/contacts/nudgedismissal.go:recordNudgeDismissal": "a rep setting a lapsed contact aside so their own decay lane stops raising them, or putting them back. An OCCURRENCE and not a replacement: NO field of the contact changes — the contact is exactly as they were, and what moved is a row in a side table saying one reader decided not to chase them for a while. The contact is the subject because the judgement is about them and is what a reader investigating why a contact stopped appearing would look up. What it has instead of a prior state is the action and, on a dismissal, the moment it lifts; the side table carries set_by and set_at, so who decided and when is recoverable from the row the audit row points at. A restore names no moment because it takes effect now, and the moment it ends was already published by the dismissal it undoes. A RE-dismissal is the one case that does replace something — a deadline that was still standing — and it takes the other branch, imaging the moment it displaced through storekit.Audit",

	"internal/modules/notices/store.go:MarkRead": "the recipient settling their notice. An OCCURRENCE and not a replacement: read_at moves from its one legal prior state (NULL — the statement's own guard makes a second settle a no-op that audits nothing), so the before-image is fully implied by the action itself, and the evidence records the one fact that changed (read: true)",

	"internal/modules/contacts/company_vat_check.go:RequestVatCheck": "a contact asking for the register to be consulted again. An OCCURRENCE and not a replacement: nothing about the company changed — the question has been asked and not yet answered — so no field holds a prior value for an image to name. It is recorded at all because it is recordable nowhere else: the worker that answers runs under a confined system principal, so the receipt it later writes says system:vatcheck whatever prompted it, and without this row nobody can tell that a contact spent a consultation on this company. The verb is `update` on `company` because that is the row the caller was authorized against and the row whose standing the answer will change",

	"internal/modules/contacts/company_vat_check.go:RecordVatCheck": "the FIRST consultation of a company's VAT ID. An OCCURRENCE and not a replacement: nothing about the company changed, an answer that did not exist before was recorded, and there is no prior verdict, receipt or register name for an image to name. A RE-check takes the other branch and carries both — which is the case that matters, because a number that was valid and now is not is the finding this lane exists for. The verb is `update` on `company` because that is the row the caller was authorized against and the row whose standing this changes",

	"internal/modules/comms/bounce.go:RecordBounce": "the receiving mail system refusing a message is an OCCURRENCE on the sent activity, not a field replacement: the bounce columns were NULL on every row this can match (the CAS refuses a second mark), so the prior state is the absence the mark ends, and there is nothing for a before-image to name",

	"internal/modules/contacts/channelidentity.go:auditChannelIdentityChange": "binding an account to a contact who had none replaces nothing: the question is which account reaches this human, and before the bind none did. A rebind takes the other branch and names the binding it moved",

	"internal/modules/contacts/impersonationwarning.go:ClearImpersonationWarning": "a user answering the sender check's warning on a contact as fine. It is an OCCURRENCE on the contact: the verdict and its reason stay on the row and gain the moment and the user that cleared it, and the image names the reason cleared",
	"internal/modules/contacts/acquisitionrequest.go:RetractAcquisitionRequest":   "a human taking back the request a contact's acquisition records. It is an OCCURRENCE on the contact: the acquisition rows stay and gain the moment the request was withdrawn, and the image names the request kinds withdrawn",
	"internal/modules/consent/qualifyingevent.go:retractQualifyingSubject":        "a human taking back an exchange they recorded. It is an OCCURRENCE on the contact, as recording is: the exchange row stays and gains the moment it was withdrawn, and the image names the exchange by its kind and when it happened",
	"internal/modules/consent/qualifyingevent.go:writeExchangeTx":                 "a human writing down an exchange that happened away from every system — a card handed over at a trade fair. It is an OCCURRENCE and not a replacement: nothing about the contact changed state, a row saying what happened was added, and there is no prior version of a meeting for the image to name. The verb is `update` on `contact` because that is the row the caller was authorized against and the row whose sendability this changes; the alternative, a create on the qualifying-event row, would put a rule in the trail that authorized nothing here",

	"internal/modules/consent/submissionreview.go:markResolvedTx": "a colleague answering what a subject proposed. An OCCURRENCE and not a replacement: the submission row went from undecided to decided, which is a fact arriving rather than a field of anybody's record changing — and on a rejection nothing about the contact moves at all. Where an ACCEPTED correction does move a field, that change is written by contacts' own update path, which records its own before-image of the value it replaced; a second image here would describe the same edit from a row that never held it. The verb is `update` on `contact` because that is the row a later reader asks about, the same choice stageSubmission makes. What it carries instead of a prior state is the kind of proposal and the decision recorded, which is what an auditor asking what we did about this contact's request needs. The proposed value stays OFF the payload for stageSubmission's reason: a second copy of the subject's own data would outlive the erasure that clears the first",

	"internal/modules/consent/confirmsubmit.go:stageSubmission": "a data subject proposing a correction to their own record through the confirm link, or asking to be removed. An OCCURRENCE and not a replacement: nothing about the contact changed state, and the field a correction names still holds its old value — deliberately, because a bearer-token caller may not write the CRM, so there is no prior version for an image to record. The verb is `update` on `contact` because that is the row a later reader asks about. The proposed value stays OFF the audit payload, where a second copy of the subject's own data would outlive the erasure that clears the first",

	// Three writes whose FIRST occurrence has no prior state and whose later
	// ones do. Each routes on that, so the branch that replaced something says
	// what, and the branch that replaced nothing says so rather than inventing
	// an image. The runtime refusal found all three; no static reading could,
	// because each reaches the door through a verb it does not spell.
	"internal/modules/capture/freemaildomain.go:AddConfirmed":                   "a carve-out the list did not hold replaces no kind, because the domain was not on it; amending an entry takes the other branch and records the kind it replaced",
	"internal/modules/capture/owndomainstore.go:Add":                            "registering a domain nobody had seen adds an entry to the workspace list, so there is no prior source or verification for the row to name; confirming a candidate a mailbox already saw takes the other branch and records what moved",
	"internal/modules/contacts/domainadmission.go:recordHumanDomainAdmissionTx": "a first decision replaces no admission, no reason and nobody answerable for one; every later decision moved all three and records what they were",

	"internal/modules/contacts/linkedinmatchapply.go:auditLinkedInHandleGained": "a gained LinkedIn handle lands in contact_social and no column of the human moves, so what the contact gained is the whole of what this write has to record. Both writers of the slot land here — the confirmed match and the empty-slot fill — so the waiver covers one function rather than following each caller",

	"internal/modules/capture/senderoverride.go:Set":                  "the settings row has no column for a sender decision; the write records one seat's answer about one address, and the image names the decision and the kind it overruled — the prior state a reader wants is what the MACHINE had said, which the image carries",
	"internal/modules/capture/senderoverride.go:Remove":               "the same decision withdrawn: what it had been is in the row this deletes, and the image names that it was withdrawn",
	"internal/modules/capture/exclusionstore.go:Add":                  "the settings row has no column for a rule; the write inserts one exclusion into a list, and the image names the rule that now applies",
	"internal/modules/capture/owneridentityrestore.go:RestoreRemoval": "the restore re-inserts the claim a withdrawal removed, so there is no prior state to record: the before-image is the withdrawal's own audit row, which this one names as the entry it reverses. The image carries the claim's id and kind and never its value, for the reason Add gives",
	"internal/modules/capture/owneridentitystore.go:AddConfirmed":     "the settings row has no column for a seat's own addresses; the write inserts one claim into a per-seat list. The image names the claim's id and kind and deliberately NOT its value — an owner identity is one contact's private address, and the audit log is where nothing erases it and every admin reads it",

	"internal/modules/contacts/providerclaims.go:WriteProviderClaims": "the bought values land in contact_provider_claim and the contact row is untouched, and quoting them would put a second copy of the subject's data in a table the erasure treats as evidence rather than as subject data",
	"internal/modules/contacts/researchclaim.go:SaveResearchClaims":   "the claims land in contact_profile_field and the contact row is untouched; what arrived rides the evidence column, where field history will not read it as a field of the record",

	"internal/modules/contacts/companytechnical.go:auditTechnicalEnrichment": "what a company publicly runs lands in company_fact and no column of the company moves, so no field has a prior value for an image to name. What was replaced is still recorded, and more precisely than an image would: the evidence carries the rows written AND the rows the reconciliation removed, which is what makes a mail provider moving to Microsoft 365 readable as a move rather than as an arrival",

	"internal/modules/collections/tags.go:auditTagLink": "the tag row is untouched; the write inserts or deletes a " +
		"taggable link, and the after image names the record. The helper is the subject because its signature admits " +
		"nothing but a tagging image on a tag, so a new caller can only write the shape ratified here.",
	"internal/modules/collections/memberwrite.go:recordMemberChange": "the list row is untouched; the write adds or " +
		"removes a Shortlist membership, and the after image names the record that joined or left.",
	"internal/modules/webforms/storechange.go:RotateSecret": "the new form secret replaces a vault reference to a value " +
		"that must never be copied into audit_log, so the after image carries the fact of the rotation and neither secret.",
	"internal/modules/webhooks/store.go:RotateSecret": "the new signing secret replaces a value that must never be " +
		"copied into audit_log, so the after image carries the fact of the rotation and neither secret.",
	"internal/modules/webhooks/deliverystore.go:requireReplay": "the subscription is unchanged; the write records that " +
		"a delivery was re-attempted, and the after image names which one.",
	"internal/modules/activities/extractionclaim.go:FinishExtractionRead": "the claim proves the reading was running, " +
		"so a prior state exists — it is a job's own progress rather than a field a contact set, and a document is " +
		"re-read by starting a new attempt, never by putting a finished one back to running.",
	"internal/modules/activities/transcriptreadfinish.go:finishTranscriptReadTx": "the compare-and-set proves the reading was " +
		"running, so this is a run record's outcome, not a record whose fields a user edits: nothing would ever be " +
		"restored to a reading's earlier progress.",
	"internal/modules/ai/feedback.go:Record": "the upsert as readily creates the ledger row as replaces one, so the " +
		"write records that a human decided rather than a field moving; a verdict it superseded was written through " +
		"this same door and stays readable as that write's own audit row.",
})

// unresolvableAuditActions: call sites whose action argument this walk cannot
// reduce to a constant, keyed by `path:function`. Each entry states that
// wrapper's own guarantee, because the gate cannot judge the verb and the
// chokepoint is what actually holds these.
//
// Ratifying them is the only honest third answer. Failing them would make the
// gate impossible to green over wrappers that are correct; merely counting them
// would let a real defect through in silence.
var unresolvableAuditActions = gatekit.Waive(map[string]string{
	"internal/modules/reporting/persistence.go:recordChange": "The shared mutation writer requires a non-nil before image for every non-create action before invoking Audit.",
	// The seam that routes an extension's own change, choosing the door from
	// what the change carries rather than from a verb it cannot read.
	"internal/compose/extledger.go:recordExtensionChange": "the verb is the unit's own, and the seam picks the door from whether the change declared a before-image, so an update either carries one or is recorded as the occurrence it says it is",

	// Wrappers whose images are computed by the caller and handed in whole.
	"internal/modules/capture/lifecycleaudit.go:auditLifecycle":                         "both images are parameters, so the verb and what it changed arrive together from a caller that has the row",
	"internal/modules/capture/sharedmailboxaudit.go:announceSharedMailboxTx":            "both images are parameters, and every caller reads the shared row under lock first, so an update always carries its prior readers and visibility",
	"internal/modules/contacts/relationshipimage.go:emitRelationshipNarrowedImagePairs": "create and archive carry the edge whole and update carries the columns that moved, narrowed against the row read under the lock",
	"internal/modules/commissions/decide.go:decideTx":                                   "the verb names the decision and the images are the patch's own, so what the decision moved is recorded with it",
	"internal/modules/commissions/decide.go:voidOne":                                    "a void is spelled as its own verb and carries the patch images for the row it retired",
	"internal/platform/settings/delete.go:DeleteRawTx":                                  "each setting declares its own verb; the before image is the stored row it removes, and the after image is the registered default the key reads as once the row is gone",
	"internal/platform/settings/store.go:SetRawTxReceipt":                               "each setting declares its own verb, and the value on either side is rendered by the same declaration",

	// The one site no static reading could ever judge, and the reason the
	// chokepoint is not optional.
	"internal/modules/privacy/retentionactions.go:auditRetentionAction": "the verb is a column of the policy row being applied, read at run time; retention_policy_action_check admits only archive, anonymize and erase, so this site cannot reach update at all",
})

func TestEveryAuditedUpdateRecordsWhatItChangedFrom(t *testing.T) {
	t.Parallel()
	defer eventShapedUpdates.AssertAllMatched(t)
	defer unresolvableAuditActions.AssertAllMatched(t)

	census := auditEntityCensusOf(t)
	for _, at := range census.directInserts {
		t.Errorf("%s writes audit_log with its own statement, so neither this gate's rule nor "+
			"storekit's refusal reaches it.\n\tCall a storekit door.", at)
	}
	var withBefore, eventShaped, unresolvable int
	for _, site := range auditDoorSitesOf(t) {
		switch beforeImageVerdictOf(site) {
		case verdictUnreadVerb:
			unresolvable++
			if !unresolvableAuditActions.Waived(t, site.key()) {
				t.Errorf("%s: %s builds its audit verb at runtime, so this gate cannot judge it.\n"+
					"\tRatify it in unresolvableAuditActions with the guarantee that holds it, "+
					"or pass the verb as a constant.", site.key(), site.door.name)
			}
		case verdictImaged:
			withBefore++
		case verdictNoBeforeImage:
			withBefore++
			t.Errorf("%s: audits an update to %s with no before-image.\n"+
				"\tRecord what the fields held (the row is readable in this transaction), "+
				"or call storekit.AuditEvent and say here what this write has instead.",
				site.key(), site.entityType())
		case verdictOccurrence:
			eventShaped++
			if !eventShapedUpdates.Waived(t, site.key()) {
				t.Errorf("%s: records an update to %s as an occurrence.\n"+
					"\tRatify it in eventShapedUpdates with what this write has instead of a "+
					"prior state, or record the before-image and call storekit.Audit.",
					site.key(), site.entityType())
			}
		case verdictNotAnUpdate:
		}
	}

	// Reported rather than fatal, so the findings above still print. Each
	// floor sits below the real count, so a reader that stopped working fails.
	if withBefore < 80 || eventShaped+unresolvable < 15 || !census.seedInserts {
		t.Errorf("resolved %d image-carrying, %d occurrence-shaped and %d runtime-verb audit site(s), "+
			"and the seed's INSERT read %v: one of the ways of finding a write has stopped working",
			withBefore, eventShaped, unresolvable, census.seedInserts)
	}
}

// beforeImageVerdict is what this gate concludes about one door site.
type beforeImageVerdict int

const (
	verdictNotAnUpdate beforeImageVerdict = iota
	verdictUnreadVerb
	verdictImaged
	verdictNoBeforeImage
	verdictOccurrence
)

// beforeImageVerdictOf judges one site. archive, create, erase and the other
// verbs are out of this gate's scope.
func beforeImageVerdictOf(site auditDoorSite) beforeImageVerdict {
	verbs, known := site.verbs()
	switch {
	case !known:
		return verdictUnreadVerb
	case !slices.Contains(verbs, "update"):
		return verdictNotAnUpdate
	case site.door.before < 0:
		return verdictOccurrence
	case site.beforeIsAbsent():
		return verdictNoBeforeImage
	}
	return verdictImaged
}

func TestAnUpdateWithANilBeforeImageIsReportedAndOneWithAnImageIsNot(t *testing.T) {
	t.Parallel()
	census := fixtureCensus(t, fixtureSeed, map[string]string{"a.go": fixtureHeader +
		"func absent() { _ = k.Record(ctx, tx, \"update\", \"alpha\", nil, map[string]any{\"a\": 1}) }\n" +
		"func imaged() { _ = k.Record(ctx, tx, \"update\", \"alpha\", map[string]any{\"a\": 0}, map[string]any{\"a\": 1}) }\n"})
	sites, _ := census.doorSites()
	got := map[string]beforeImageVerdict{}
	for _, site := range sites {
		got[site.use.decl.Name.Name] = beforeImageVerdictOf(site)
	}
	if got["absent"] != verdictNoBeforeImage || got["imaged"] != verdictImaged {
		t.Errorf("judged %v, want absent reported and imaged passed", got)
	}
}

func TestAnExportedWriterWhoseVerbNoRuleReadsIsUnjudged(t *testing.T) {
	t.Parallel()
	census := fixtureCensus(t, fixtureSeed, map[string]string{"a.go": fixtureHeader})
	_, unjudged := census.doorSites()
	if len(unjudged) != 1 || !strings.HasSuffix(unjudged[0], ".FromData") {
		t.Errorf("unjudged %v, want only the planted writer that reads its verb from data", unjudged)
	}
}
