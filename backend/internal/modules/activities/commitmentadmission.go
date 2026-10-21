// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CommitmentAdmissionVersion identifies the policy used by extraction and reviewed repair.
const CommitmentAdmissionVersion = "2"

// AutomaticMailContent is the content boundary shared by extraction and its admission check.
const AutomaticMailContent = `audience = 'workspace' AND restricted_at IS NULL`

// CommitmentAdmissionReason explains whether evidence supports current automatic work.
type CommitmentAdmissionReason string

// Admission reasons distinguish history and missing evidence from completed work.
const (
	CommitmentCurrent           CommitmentAdmissionReason = ""
	CommitmentHistorical        CommitmentAdmissionReason = "historical_without_current_deadline"
	CommitmentIncomplete        CommitmentAdmissionReason = "later_context_not_read"
	CommitmentContextUnassessed CommitmentAdmissionReason = "later_context_not_assessed"
	CommitmentSourceUnavailable CommitmentAdmissionReason = auth.CommitmentEvidenceUnavailable
	CommitmentSourceUnknown     CommitmentAdmissionReason = "source_time_unknown"
	CommitmentTimeContradicted  CommitmentAdmissionReason = "source_time_contradicted"
	CommitmentMeetingCancelled  CommitmentAdmissionReason = "meeting_cancelled"
	CommitmentMeetingPending    CommitmentAdmissionReason = "meeting_not_yet_held"
	CommitmentBeforeBaseline    CommitmentAdmissionReason = "before_mailbox_history_baseline"
)

// CommitmentAdmissionInput names the source and the context actually read.
type CommitmentAdmissionInput struct {
	Preview      bool
	SourceID     ids.ActivityID
	ReadMessages []ids.UUID
}

// CommitmentAdmissionEvidence carries source facts; decision reads hold the source row lock.
type CommitmentAdmissionEvidence struct {
	RawCaptureID       *ids.UUID
	CapturedMail       bool
	OccurredAt         time.Time
	ProviderReceivedAt *time.Time
	SourceOccurredAt   *time.Time
	UnreadLater        bool
	ContextAssessed    bool
	CancelledMeeting   bool
	Meeting            bool
}

// AdmitAutomaticCommitment does not change existing work or assert that a promise was fulfilled.
func AdmitAutomaticCommitment(evidence CommitmentAdmissionEvidence, asOf time.Time, dueAt *time.Time) CommitmentAdmissionReason {
	if asOf.IsZero() || evidence.OccurredAt.IsZero() {
		return CommitmentSourceUnknown
	}
	if evidence.CancelledMeeting {
		return CommitmentMeetingCancelled
	}
	if evidence.OccurredAt.After(asOf) {
		if evidence.Meeting {
			return CommitmentMeetingPending
		}
		return CommitmentSourceUnknown
	}
	cutoff := asOf.AddDate(0, 0, -provenance.CommitmentHorizonDays)
	if arrival := evidence.ProviderReceivedAt; arrival != nil {
		if evidence.OccurredAt.After(arrival.Add(24*time.Hour)) || (evidence.OccurredAt.Before(cutoff) && !arrival.Before(cutoff)) {
			return CommitmentTimeContradicted
		}
	}
	if evidence.UnreadLater {
		if !evidence.ContextAssessed {
			return CommitmentContextUnassessed
		}
		return CommitmentIncomplete
	}
	if evidence.OccurredAt.Before(cutoff) && (dueAt == nil || deadline.Passed(dueAt, cutoff)) {
		return CommitmentHistorical
	}
	return CommitmentCurrent
}

// CommitmentAdmissionEvidenceTx locks source facts for decisions, but keeps previews read-only.
// A nil read set leaves later context unassessed; repair must not treat it as evidence of fulfillment.
func CommitmentAdmissionEvidenceTx(ctx context.Context, tx pgx.Tx, in CommitmentAdmissionInput) (CommitmentAdmissionEvidence, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return CommitmentAdmissionEvidence{}, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, in.SourceID.UUID); err != nil {
		return CommitmentAdmissionEvidence{}, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT a.occurred_at, a.raw_capture_id, a.kind = 'email' AND a.source <> 'manual', a.kind = 'meeting', coalesce(a.meeting_status = 'canceled', false),
  (SELECT min(ci.source_occurred_at) FROM capture_import ci WHERE ci.activity_id = a.id),
  (SELECT min(ci.provider_received_at) FROM capture_import ci WHERE ci.activity_id = a.id),
  a.kind = 'email' AND EXISTS (SELECT 1 FROM activity later
   WHERE later.kind = a.kind AND later.thread_key = a.thread_key
    AND later.archived_at IS NULL AND `+AutomaticMailContent+`
    AND (later.occurred_at, later.id) > (a.occurred_at, a.id)
    AND NOT coalesce(later.id = ANY($%[1]d::uuid[]), false))
 FROM activity a WHERE a.id = $%[2]d`, arg(in.ReadMessages), arg(in.SourceID))
	if !in.Preview {
		query += " FOR UPDATE OF a"
	}
	var evidence CommitmentAdmissionEvidence
	if err := tx.QueryRow(ctx, query, args...).Scan(&evidence.OccurredAt, &evidence.RawCaptureID, &evidence.CapturedMail, &evidence.Meeting, &evidence.CancelledMeeting, &evidence.SourceOccurredAt, &evidence.ProviderReceivedAt, &evidence.UnreadLater); err != nil {
		return CommitmentAdmissionEvidence{}, fmt.Errorf("activities: reading commitment admission evidence: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, in.SourceID.UUID); err != nil {
		return CommitmentAdmissionEvidence{}, err
	}
	evidence.ContextAssessed = in.ReadMessages != nil
	return evidence, nil
}
