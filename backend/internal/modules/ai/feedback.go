// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The correction ledger: what a human has already decided about a claim the
// system derives (AIRT-SCHEMA-1, AIRT-AC-9).
//
// Everything inferred here is re-derived rather than stored — a brief line, a
// moment, an enriched field. That is what keeps it honest, and it is also what
// makes it forget: correct one, and the next read asserts the same wrong thing
// again, because nothing remembers that a human already answered.
//
// The claim KEY is what makes remembering work. It is a hash of the claim's
// normalized path — what the claim is about — and never of its value. Keyed on
// the value a verdict would evaporate the moment the evidence shifted, which
// is precisely when the human's answer matters most.
//
// The ledger stores the verdict and never the model's asserted value. Nothing
// is gained by keeping what was rejected, and a rejected assertion sitting in
// a table is one bad join away from being shown again.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Claim kinds, as the column's CHECK constrains them.
const (
	ClaimProfileField  = "profile_field"
	ClaimInferredKPI   = "inferred_kpi"
	ClaimNextStep      = "next_step"
	ClaimSignal        = "signal"
	ClaimResearchClaim = "research_claim"
)

// Verdicts. Each one changes what a later re-derivation is allowed to do.
const (
	// VerdictSuppressed: never surface this claim again.
	VerdictSuppressed = "suppressed"
	// VerdictCorrected: show the human's value, and never overwrite it with a
	// fresh inference without a recorded 🟡 approval.
	VerdictCorrected = "corrected"
	// VerdictConfirmed: the claim stands and may carry a "confirmed by" marker.
	VerdictConfirmed = "confirmed"
)

// fieldSubjectType names the input a refusal points at, spelled once so the
// two refusal sites cannot disagree about which field the caller must fix.
const fieldSubjectType = "subject_type"

// Subject types the ledger accepts, matching the column's CHECK. They are also
// the RBAC objects the write is gated on: correcting what the system says
// about a contact requires the grant to edit that contact.
var feedbackSubjects = map[string]bool{
	"organization": true,
	"person":       true,
	"deal":         true,
	"lead":         true,
}

// FeedbackStore owns the ledger.
type FeedbackStore struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
}

// NewFeedbackStore binds the ledger to the pool. It holds no other
// dependency: consulting a verdict is a read of one table, and recording one
// is a decision a human has already made.
func NewFeedbackStore(db *database.DB) *FeedbackStore { return &FeedbackStore{db: db} }

// ClaimKey is the stable identity of a logical claim within a subject and
// kind: a hash of the claim's normalized PATH, never its value.
//
// Callers pass the path they would use to name the claim to a colleague —
// "profile_field:title", "moment:replied_after_gap". Normalizing here rather
// than trusting the caller is what stops "Title" and "title" becoming two
// claims, which would let a correction be lost to a capitalization.
func ClaimKey(path string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(path))))
	return hex.EncodeToString(sum[:])
}

// Verdict is one recorded human decision, as the ledger holds it.
//
// What the human SAID is reached through AsOf and nowhere else, and the fields
// carrying it are unexported for that reason. A decision is about the value
// that was in front of the person who made it; a surface that could read it
// without saying which version of the value it is asking about would serve a
// correction over an answer the human never saw. That was a live defect on the
// 360 page, and unexported fields are what stops it being written again.
type Verdict struct {
	ClaimKind string
	ClaimKey  string

	// recordedAt is ai_feedback.updated_at — when this decision took its
	// current form, which the upsert moves on every re-decision. NOT
	// created_at: a human who changes their mind is looking at whatever is
	// there now, and dating their new decision from their old one would make
	// it stale the moment they made it.
	recordedAt     time.Time
	verdict        string
	correctedValue *string
	note           *string
}

// Decision is what a human decided about one claim, as it applies to a
// particular version of the value.
type Decision struct {
	Verdict        string
	Note           *string
	CorrectedValue *string
}

// AsOf answers what a human decided about a value last written at valueAt, and
// whether their decision applies to that value at all.
//
// A verdict recorded BEFORE the value it is read against is about something
// else. The human was looking at an earlier answer and something has replaced
// it since — an accepted research claim, a fresh enrichment, an edit through
// another door — so their correction describes a value that is no longer
// stored. Serving it tells the reader the current value is one the record does
// not hold.
//
// The MARKER goes with it, and that is the half worth stating. Keeping
// "corrected" beside the newer stored value would say the human wrote that
// value; keeping "confirmed" would say they had seen it. Both are the same lie
// one field along, and a reader has no way to tell. What a human once decided
// is still in the ledger and in audit_log for anyone asking that question; what
// this answers is the narrower one the page asks — does their decision describe
// what is on the screen.
//
// Equal timestamps STAND. A verdict and the value it is about can be written in
// one transaction, where both carry the same now(), and refusing that case
// would suppress a decision at the instant it was made.
func (v Verdict) AsOf(valueAt time.Time) (Decision, bool) {
	if v.recordedAt.Before(valueAt) {
		return Decision{}, false
	}
	return Decision{Verdict: v.verdict, Note: v.note, CorrectedValue: v.correctedValue}, true
}

// NewVerdict builds one. It exists because the fields above are unexported: the
// store that reads the ledger and the tests that drive the ruling need a way to
// set them, and a constructor is a door the recency question cannot slip past.
func NewVerdict(claimKind, claimKey, verdict string, correctedValue, note *string, recordedAt time.Time) Verdict {
	return Verdict{
		ClaimKind: claimKind, ClaimKey: claimKey, verdict: verdict,
		correctedValue: correctedValue, note: note, recordedAt: recordedAt,
	}
}

// RecordInput is a human's decision about one claim.
type RecordInput struct {
	SubjectType string
	SubjectID   ids.UUID
	ClaimKind   string
	// ClaimPath is the claim's normalized path; the store hashes it. Callers
	// pass the path rather than a hash so the hashing rule has exactly one
	// implementation and cannot drift per surface.
	ClaimPath      string
	Verdict        string
	CorrectedValue *string
	Note           *string
}

// Record writes a human's verdict, replacing any previous one for the same
// claim.
//
// Replacing rather than appending is the point: a verdict is the current
// answer to "has a human decided this", and two answers is no answer. The
// history is in audit_log, where every other mutation's history lives.
func (s *FeedbackStore) Record(ctx context.Context, in RecordInput) error {
	if err := admitVerdict(ctx, in); err != nil {
		return err
	}
	// The one spelling for provenance, and it returns an error rather than a
	// zero value: captured_by is text NOT NULL, which "" satisfies, so a
	// principal-less path would write a verdict attributed to nobody — and
	// "corrected by you" with no you is the one thing this row must never say.
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	key := ClaimKey(in.ClaimPath)

	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Live, not merely visible: an unbounded caller skips the plain
		// probe's query entirely, so an archived or erased subject would keep
		// accruing verdicts — and a corrected_value is human-typed text about
		// them that the profile-fields read would then render.
		if err := auth.HoldWritableLive(ctx, tx, in.SubjectType, in.SubjectID); err != nil {
			return err
		}
		// And held, because ai_feedback is a declared PII table that Art. 17
		// erasure deletes: a verdict written after that commit restores
		// human-typed text ABOUT the subject to a table the erasure had
		// cleared. The probe above narrows the window; this closes it.
		//
		// SubjectType is the polymorphic arm, and it is safe as an identifier
		// for the same reason the probe above takes it: both are checked
		// against the closed feedbackSubjects vocabulary by admitVerdict, which runs
		// before the transaction opens.
		id, err := upsertVerdict(ctx, tx, in, key, capturedBy)
		if err != nil {
			return err
		}
		// Audit-only: the closed catalog (events.md §5) defines no
		// ai_feedback.* type, and the closed-verb law forbids inventing one
		// build-side. Nothing downstream reacts to a verdict either — the
		// ledger is CONSULTED at re-derivation time — so the audit row carries
		// the attribution a "corrected by you" marker and any later dispute
		// both need.
		//
		// The after-image records the claim and the verdict, never the value
		// the model asserted: the ledger does not keep that and neither does
		// its audit trail.
		//
		// AuditEvent, not Audit: the upsert as readily creates the ledger row as
		// replaces one, so what this records is that a human decided, not a
		// field moving. Where a verdict did stand, it was written through this
		// same door and its own audit row is where it is still readable.
		_, err = storekit.AuditEvent(ctx, tx, "update", "ai_feedback", id, map[string]any{
			"subject_type": in.SubjectType,
			"subject_id":   in.SubjectID.String(),
			"claim_kind":   in.ClaimKind,
			"claim_key":    key,
			"verdict":      in.Verdict,
		})
		return err
	})
}

// admitVerdict refuses what must never reach the ledger, naming the field the
// caller has to fix rather than letting a column CHECK answer for it.
func admitVerdict(ctx context.Context, in RecordInput) error {
	if !feedbackSubjects[in.SubjectType] {
		return &values.ParseError{
			Field: fieldSubjectType, Code: "invalid_subject_type",
			Message: "a claim is about an organization, person, deal or lead",
		}
	}
	if strings.TrimSpace(in.ClaimPath) == "" {
		return &values.ParseError{
			Field: "claim_path", Code: "missing_claim_path",
			Message: "a verdict names the claim it is about",
		}
	}
	// A corrected verdict without a value is a human's answer that was lost on
	// the way in. The database refuses it too; refusing here names what is
	// missing instead of surfacing a constraint.
	if (in.Verdict == VerdictCorrected) != (in.CorrectedValue != nil) {
		return &values.ParseError{
			Field: "corrected_value", Code: "corrected_value_mismatch",
			Message: "a corrected verdict carries the human's value, and no other verdict does",
		}
	}
	// Human-only, because the whole point of the row is that a PERSON decided:
	// the column is hard-coded source = 'human', and a verdict an agent could
	// write would let a model launder its own claim into the ledger that is
	// supposed to overrule it.
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	// Gated on the SUBJECT's own grant rather than on a new object of its own:
	// correcting what the system says about a contact is editing that contact,
	// and a separate object would let the two drift apart.
	return auth.Require(ctx, in.SubjectType, principal.ActionUpdate)
}

// upsertVerdict replaces any previous decision about the same claim. Replacing
// rather than appending is the point: a verdict is the current answer to "has a
// human decided this", and two answers is no answer. The history is in
// audit_log, where every other mutation's history lives.
func upsertVerdict(ctx context.Context, tx pgx.Tx, in RecordInput, key, capturedBy string) (ids.UUID, error) {
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO ai_feedback
		  (subject_type, subject_id, claim_kind, claim_key,
		   verdict, corrected_value, note, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'human', $8)
		ON CONFLICT (subject_type, subject_id, claim_kind, claim_key)
		DO UPDATE SET verdict = EXCLUDED.verdict,
		              corrected_value = EXCLUDED.corrected_value,
		              note = EXCLUDED.note,
		              captured_by = EXCLUDED.captured_by,
		              version = ai_feedback.version + 1,
		              updated_at = now()
		RETURNING id`,
		in.SubjectType, in.SubjectID, in.ClaimKind, key,
		in.Verdict, in.CorrectedValue, in.Note, capturedBy,
	).Scan(&id); err != nil {
		return ids.Nil, fmt.Errorf("ai: recording a human's verdict on a derived claim: %w", err)
	}
	return id, nil
}

// VerdictsForTx returns every verdict recorded about one record, keyed by
// "<claim_kind>:<claim_key>".
//
// One read per record rather than per claim: a page re-derives many claims
// about the same subject, and asking the ledger per line would be a query per
// rendered sentence.
func (s *FeedbackStore) VerdictsForTx(ctx context.Context, tx pgx.Tx, subjectType string, subjectID ids.UUID) (map[string]Verdict, error) {
	if !feedbackSubjects[subjectType] {
		return nil, &values.ParseError{
			Field: fieldSubjectType, Code: "invalid_subject_type",
			Message: "a claim is about an organization, person, deal or lead",
		}
	}
	// A read grant on the subject, matching the read this consult decorates.
	// The caller has already resolved the subject's row scope by the time it
	// asks what a human decided about it.
	if err := auth.Require(ctx, subjectType, principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		-- updated_at, not created_at: it is when this decision took its
		-- CURRENT form, and a reader compares it against when the value took
		-- its own. See Verdict.AsOf.
		SELECT claim_kind, claim_key, verdict, corrected_value, note, updated_at
		  FROM ai_feedback
		 WHERE subject_type = $1 AND subject_id = $2`, subjectType, subjectID)
	if err != nil {
		return nil, fmt.Errorf("ai: reading the verdicts recorded about a record: %w", err)
	}
	defer rows.Close()

	out := map[string]Verdict{}
	for rows.Next() {
		var claimKind, claimKey, verdict string
		var correctedValue, note *string
		var recordedAt time.Time
		if err := rows.Scan(&claimKind, &claimKey, &verdict, &correctedValue, &note, &recordedAt); err != nil {
			return nil, fmt.Errorf("ai: reading a recorded verdict: %w", err)
		}
		out[VerdictLookupKey(claimKind, claimKey)] = NewVerdict(claimKind, claimKey, verdict, correctedValue, note, recordedAt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai: reading the recorded verdicts: %w", err)
	}
	return out, nil
}

// VerdictLookupKey is how a consulting surface finds one claim's verdict in
// the map VerdictsForTx returns. It exists as a named function so the two
// sides cannot spell the composite key differently.
func VerdictLookupKey(claimKind, claimKey string) string {
	return claimKind + ":" + claimKey
}
