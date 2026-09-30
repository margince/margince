// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pipelinetrace

// Reason is WHY a stage reached the status it did — a class this installation
// chose, never a provider's text and never message content.
//
// That restriction is what makes the default posture sufficient. With payload
// capture off the trace stores no address and no subject, so if a skip could
// only be explained by quoting the message, every installation running the
// default would read "we cannot tell you" — which is the state this surface
// exists to remove. A class renders a full sentence from the catalog with no
// payload at all.
//
// SCOPED BY STAGE. The catalog key is `<stage>.<reason>`, so the same code says
// something stage-appropriate in each place it appears: ReasonTransportNotRead
// under the classifier is "reads email only", and under the conversation reader
// it is a different sentence about a different pass. Without the scope the two
// would need differently-spelled constants saying nearly the same thing, and the
// pair would drift.
type Reason string

// Stored reasons. These are capture's own vocabulary, already written to
// capture_trace.reason by the three writers, and are listed here so the registry
// can close the set the surface may render.
const (
	// StageInternalDrop.
	ReasonInternalOnly Reason = "internal_only"

	// StageActivityWrite.
	ReasonInvisibleIncumbent Reason = "invisible_incumbent"

	// StageTierLadder.
	ReasonTransactionalInfra  Reason = "transactional_infra"
	ReasonTransactionalPrefix Reason = "transactional_prefix"
	ReasonDeferralCapped      Reason = "deferral_capped"
	ReasonNoisePrior          Reason = "noise_prior"
	ReasonDecidedPrior        Reason = "decided_prior"
	ReasonNoCounterparty      Reason = "no_counterparty"
	ReasonNoGrantingHuman     Reason = "no_granting_human"
	ReasonDerivationFailed    Reason = "derivation_failed"
)

// Derived reasons. These are computed at read time from live product state, and
// exist so a stage that did NOT run says why rather than rendering as an absence.
const (
	// StageContactCreate. Neither promises a repair: the link_reconcile sweep
	// links a message once a contact EXISTS for its address and never re-runs
	// the resolver, but a channel identity
	// conflict stages a human-review proposal the resolver will never clear, so
	// that message stays link-less until a contact acts. Copy that said "tonight"
	// would be false indefinitely for exactly those.
	ReasonNotLinkedYet      Reason = "not_linked_yet"
	ReasonNoContactIntended Reason = "no_contact_intended"

	// Every stored stage, when this surface cannot establish what happened.
	// One class for both causes, because the two are indistinguishable once the
	// rows are gone: a swept window and a reader who does not own the rows
	// produce the same absence, and naming either specifically would state a
	// fact we do not have — or, for a non-owner, disclose one we must not.
	ReasonRecordNotAvailable Reason = "record_not_available"

	// StageVerdict.
	ReasonAwaitingVerdict Reason = "awaiting_verdict"
	ReasonNoOpenQuestion  Reason = "no_open_question"
	// The verdict itself, one reason per answer. A single `verdict_reached`
	// told a member that the one fact they opened the panel for exists, and
	// not what it was — the same failure StatusNotReported's four kinds exist
	// to avoid, on the rung a contact came to read.
	ReasonJudgedReal       Reason = "judged_real"
	ReasonJudgedNoise      Reason = "judged_noise"
	ReasonJudgedRejected   Reason = "judged_rejected"
	ReasonJudgedSuppressed Reason = "judged_suppressed"

	// StageAttentionLabel. Six ways the backlog excludes a message, because a
	// ladder that says "reads email only" about an ARCHIVED email gives a wrong
	// why, which is worse than none.
	ReasonTransportNotRead     Reason = "transport_not_read"
	ReasonSenderUndecided      Reason = "sender_undecided"
	ReasonArchived             Reason = "archived"
	ReasonNotConnectorCaptured Reason = "not_connector_captured"
	ReasonAudienceLimited      Reason = "audience_limited"
	ReasonModelsDeclined       Reason = "models_declined"
	ReasonAwaitingBatch        Reason = "awaiting_batch"
	ReasonLabelled             Reason = "labelled"

	// StageMaterialEvents. The extractor reads a CONVERSATION, so every reason
	// here is about the thread rather than about this message — and each one is
	// an arm of the offer the extractor itself applies, named where that rule is
	// spelled so the two cannot come to mean different things.
	ReasonEventsRaised      Reason = "events_raised"
	ReasonNothingMaterial   Reason = "nothing_material"
	ReasonThreadStillMoving Reason = "thread_still_moving"
	ReasonAwaitingScan      Reason = "awaiting_scan"
	ReasonReadingParked     Reason = "reading_parked"
	ReasonNoSingleAccount   Reason = "no_single_account"
	ReasonTwoBodiesOfWork   Reason = "two_bodies_of_work"
	ReasonThreadNotAllOpen  Reason = "thread_not_all_open"
	ReasonNoNamedReader     Reason = "no_named_reader"

	// StageCompanyTriage, whose subject is a DOMAIN. These are the per-domain
	// answers the company surface reports; the message ladder carries no rung
	// for this stage at all, because a per-message ladder cannot honestly say
	// "the domain was triaged" about one message out of the hundred that shared
	// the answer.
	ReasonCompanyWarranted  Reason = "company_warranted"
	ReasonNoSiteIdentified  Reason = "no_site_identified"
	ReasonTriageQueued      Reason = "triage_queued"
	ReasonTriageUnevidenced Reason = "triage_unevidenced"
	ReasonTriageStale       Reason = "triage_stale_evidence"
	ReasonTriageNearDupe    Reason = "triage_near_duplicate"
)

// Absence reasons. Why a whole STAGE reports nothing, as against why one message
// took a particular path through it. They are catalog keys for the same reason
// every other reason is: verbatim English on a registration would render
// untranslated on a de or vi surface while the rest of the ladder was localised.
const (
	// StageConnectorFilter.
	AbsentNotComparable Reason = "not_comparable_between_connectors"
	// StageIngressGate.
	AbsentConnectorDefect Reason = "connector_side_defect"
	// StageErasureCheck.
	AbsentWouldRestoreErased Reason = "would_restore_erased"
	// StageClaimExtraction.
	AbsentNoWriterYet Reason = "no_writer_yet"
	// StageCompanyTriage on the MESSAGE ladder. It runs, and it is reported —
	// just not here: its subject is a domain, and a domain is triaged once for
	// every message that ever arrives from it. A per-message rung would answer
	// "done" for a message that prompted nothing and for the one that prompted
	// everything alike, which reads as this message having been the cause.
	AbsentAnsweredOnTheCompany Reason = "answered_on_the_company"
)
