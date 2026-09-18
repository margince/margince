// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pipelinetrace

import "sort"

// Source is where a stage's answer comes from.
//
// A stage declares a SET of these, not one, because StageActivityWrite is
// genuinely both: its success is derived from the activity existing, and its one
// failure mode leaves a stored row and no activity. A singular field could not
// express that, and the registry would have disagreed with the table.
type Source string

const (
	// SourceStored means capture writes a capture_trace row for this stage.
	SourceStored Source = "stored"
	// SourceDerived means a module already holds durable state that answers it.
	//
	// This is the DEFAULT preference, not a fallback. A stage that copies a
	// durable record into a trace creates a second source that can disagree with
	// the first: migration 0258 refuses to copy verdict outcomes for exactly
	// this reason, and a copy also inherits an access axis it cannot honour —
	// a job-written row lands under no member, where the ingest-written row was
	// that member's alone.
	SourceDerived Source = "derived"
	// SourceByDesign means this stage will never be reported here, and the
	// registration says why in words.
	SourceByDesign Source = "by_design"
	// SourceUntraceable means reporting it would itself breach something. The erasure
	// check is the case: writing the row re-stores what the erasure removed.
	SourceUntraceable Source = "untraceable"
	// SourcePlanned means the stage RUNS but is not reported here yet.
	//
	// Without this state the gates and any staged rollout are mutually
	// exclusive: a built-but-unreported stage is not `not_built` (a lie), not
	// `by_design` (a lie) and not stored or derived (false). It carries an issue
	// ref so it is tracked rather than excused.
	SourcePlanned Source = "planned"
	// SourceAnsweredElsewhere means the stage RUNS and IS reported — on another
	// surface, because its subject is not this ladder's.
	//
	// It is not SourceByDesign, which says the answer will never exist, and not
	// SourcePlanned, which says it does not exist yet: this one exists and has
	// a door. The distinction is the member's: told "not reported", they stop
	// looking; told where it is answered, they go there. The registration's
	// AbsentReason is what names the door, so a member reads a sentence rather
	// than an absence.
	SourceAnsweredElsewhere Source = "answered_elsewhere"
	// SourceNotBuilt means the pipeline step itself does not exist.
	SourceNotBuilt Source = "not_built"
)

// Registration is one stage's declaration of itself.
type Registration struct {
	Stage       Stage
	Order       int
	SubjectKind SubjectKind
	Sources     []Source

	// Funnel says this stage's stored rows are counted in the member-facing
	// outcome funnel and in margince_capture_outcomes_total.
	//
	// It is opt-IN, and that is the point. The metric is an in-process counter
	// incremented inside the trace writer, so a future stage writing through the
	// same writer would silently inflate it with no diff to any metric code.
	// Defaulting to false means a new stage has to say it belongs in the funnel.
	Funnel bool

	// AbsentReason explains a stage that reports nothing. Required for every
	// source except stored and derived.
	AbsentReason Reason

	// Issue is the tracking reference for a planned stage.
	Issue string

	// Reasons is the closed set this stage may carry. Closing it is what lets a
	// test assert every reason has catalog entries in every locale, and what
	// stops a value off the wire being interpolated into a key and rendered at a
	// member as `captureTrace.reason.attention_label.something`.
	Reasons []Reason
}

// registrations is the pipeline, in the order a message meets it.
//
// Order is explicit rather than positional so a stage inserted later does not
// renumber its neighbours, and gaps are deliberate: they leave room to insert
// without a sweeping edit.
var registrations = []Registration{{
	Stage:        StageConnectorFilter,
	Order:        10,
	SubjectKind:  SubjectMessage,
	Sources:      []Source{SourceByDesign},
	AbsentReason: AbsentNotComparable,
}, {
	Stage:        StageIngressGate,
	Order:        20,
	SubjectKind:  SubjectMessage,
	Sources:      []Source{SourceByDesign},
	AbsentReason: AbsentConnectorDefect,
}, {
	Stage:        StageErasureCheck,
	Order:        30,
	SubjectKind:  SubjectMessage,
	Sources:      []Source{SourceUntraceable},
	AbsentReason: AbsentWouldRestoreErased,
}, {
	Stage:       StageInternalDrop,
	Order:       40,
	SubjectKind: SubjectMessage,
	Sources:     []Source{SourceStored},
	Funnel:      true,
	Reasons:     []Reason{ReasonInternalOnly, ReasonRecordNotAvailable},
}, {
	Stage:       StageActivityWrite,
	Order:       50,
	SubjectKind: SubjectMessage,
	Sources:     []Source{SourceStored, SourceDerived},
	Funnel:      true,
	Reasons:     []Reason{ReasonInvisibleIncumbent, ReasonRecordNotAvailable},
}, {
	Stage:       StageTierLadder,
	Order:       60,
	SubjectKind: SubjectSender,
	Sources:     []Source{SourceStored},
	Funnel:      true,
	Reasons: []Reason{
		ReasonTransactionalInfra, ReasonTransactionalPrefix, ReasonDeferralCapped,
		ReasonNoisePrior, ReasonDecidedPrior, ReasonNoCounterparty,
		ReasonNoGrantingHuman, ReasonDerivationFailed, ReasonRecordNotAvailable,
	},
}, {
	Stage:       StageContactCreate,
	Order:       70,
	SubjectKind: SubjectSender,
	Sources:     []Source{SourceDerived},
	Reasons:     []Reason{ReasonNotLinkedYet, ReasonNoContactIntended, ReasonRecordNotAvailable},
}, {
	Stage:       StageVerdict,
	Order:       80,
	SubjectKind: SubjectSender,
	Sources:     []Source{SourceDerived},
	Reasons: []Reason{
		ReasonAwaitingVerdict, ReasonNoOpenQuestion, ReasonRecordNotAvailable,
		ReasonJudgedReal, ReasonJudgedNoise, ReasonJudgedRejected, ReasonJudgedSuppressed,
	},
}, {
	// Runs, and is reported — on the COMPANY, not here. Its subject is a domain,
	// and a domain is triaged once for every message that ever arrives from it:
	// a per-message rung would answer "done" for the message that prompted the
	// triage and for the hundredth one that arrived after it alike, which reads
	// as this message having been the cause. So the ladder names where the
	// answer lives instead, and the answer itself is
	// GET /companies/{id}/capture-triage.
	Stage:        StageCompanyTriage,
	Order:        90,
	SubjectKind:  SubjectDomain,
	Sources:      []Source{SourceAnsweredElsewhere},
	AbsentReason: AbsentAnsweredOnTheCompany,
	Reasons: []Reason{
		ReasonCompanyWarranted, ReasonNoSiteIdentified, ReasonTriageQueued,
		ReasonTriageUnevidenced, ReasonTriageStale, ReasonTriageNearDupe,
	},
}, {
	Stage:       StageAttentionLabel,
	Order:       100,
	SubjectKind: SubjectMessage,
	Sources:     []Source{SourceDerived},
	Reasons: []Reason{
		ReasonTransportNotRead, ReasonSenderUndecided, ReasonArchived,
		ReasonNotConnectorCaptured, ReasonAudienceLimited, ReasonAwaitingBatch,
		ReasonLabelled, ReasonRecordNotAvailable,
	},
}, {
	// Derived from the extractor's own rule: the arms it applies to decide
	// whether a conversation is read, asked of this message's thread. Its chat
	// answer is transport_not_read and that is a real answer rather than a gap
	// report — material events do not widen to chat until the grouping question
	// is settled (#1433), so a transport with no thread key has no unit of work
	// for this stage to have run over.
	Stage:       StageMaterialEvents,
	Order:       110,
	SubjectKind: SubjectThread,
	Sources:     []Source{SourceDerived},
	Reasons: []Reason{
		ReasonTransportNotRead, ReasonArchived, ReasonRecordNotAvailable,
		ReasonEventsRaised, ReasonNothingMaterial, ReasonThreadStillMoving,
		ReasonAwaitingScan, ReasonReadingParked, ReasonNoSingleAccount,
		ReasonTwoBodiesOfWork, ReasonThreadNotAllOpen, ReasonNoNamedReader,
	},
}, {
	Stage:        StageClaimExtraction,
	Order:        120,
	SubjectKind:  SubjectSender,
	Sources:      []Source{SourceNotBuilt},
	AbsentReason: AbsentNoWriterYet,
}}

// Registrations returns every stage, ordered as a message meets them.
func Registrations() []Registration {
	out := make([]Registration, len(registrations))
	copy(out, registrations)
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Lookup returns one stage's registration.
func Lookup(stage Stage) (Registration, bool) {
	for _, r := range registrations {
		if r.Stage == stage {
			return r, true
		}
	}
	return Registration{}, false
}

// StoredStages are the stages capture writes a row for, and therefore the only
// values the capture_trace.stage column may hold.
//
// TestTheStageCheckAdmitsExactlyTheStoredStages reads the live constraint with
// pg_get_constraintdef and asserts the two agree, so a migration cannot admit a
// stage the registry has never heard of, and the registry cannot name one the
// column would reject at write time.
func StoredStages() []Stage {
	var out []Stage
	for _, r := range Registrations() {
		if r.has(SourceStored) {
			out = append(out, r.Stage)
		}
	}
	return out
}

// FunnelStages are the stages whose rows the member's outcome funnel counts.
//
// The window list and the funnel both filter on this, spelled once: a row from a
// stage outside the funnel is a rung on one message's ladder, not a separate
// message, and letting it into the list would show the same message twice while
// the counters above disagreed with the rows below them.
func FunnelStages() []Stage {
	var out []Stage
	for _, r := range Registrations() {
		if r.Funnel {
			out = append(out, r.Stage)
		}
	}
	return out
}

// StageStrings renders a stage list for a SQL parameter.
func StageStrings(stages []Stage) []string {
	out := make([]string, len(stages))
	for i, s := range stages {
		out[i] = string(s)
	}
	return out
}

// CanStore reports whether capture may write a row for this stage — the same
// question the column's CHECK asks, so a writer can name the offending stage
// before the database answers with a constraint violation that names nothing.
func CanStore(stage Stage) bool {
	r, ok := Lookup(stage)
	return ok && r.has(SourceStored)
}

// CountsInFunnel reports whether this stage's rows belong in the outcome funnel
// and its metric. Unknown stages answer false: a stage nobody registered must
// not silently change what an operator is alerting on.
func CountsInFunnel(stage Stage) bool {
	r, ok := Lookup(stage)
	return ok && r.Funnel
}

func (r Registration) has(source Source) bool {
	for _, s := range r.Sources {
		if s == source {
			return true
		}
	}
	return false
}
