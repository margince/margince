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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
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
	CommitmentSourceIncomplete  CommitmentAdmissionReason = "source_text_incomplete"
	CommitmentSourceUnassessed  CommitmentAdmissionReason = "source_text_unassessed"
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
	RawCaptureID *ids.UUID
	CapturedMail bool
	OccurredAt   time.Time
	// DateOnly says OccurredAt is a day entry's anchor, which names only a date.
	DateOnly           bool
	ProviderReceivedAt *time.Time
	SourceOccurredAt   *time.Time
	UnreadLater        bool
	// SourceIncomplete is a source mail whose text was not read in full, or is
	// a forwarded original's. SourceUnassessed is one stored before that was
	// recorded and not yet checked. LaterUnassessed is the same of a later message.
	SourceIncomplete bool
	SourceUnassessed bool
	LaterUnassessed  bool
	ContextAssessed  bool
	CancelledMeeting bool
	Meeting          bool
	// LoggedByHand is evidence a colleague typed in (any kind), as opposed to
	// capture or an agent. Someone logging it today says it still matters.
	LoggedByHand bool
}

// AdmitAutomaticCommitment does not change existing work or assert that a promise was fulfilled.
// zone says which date is today for a source known only by its date.
func AdmitAutomaticCommitment(evidence CommitmentAdmissionEvidence, asOf time.Time, zone *time.Location, dueAt *time.Time) CommitmentAdmissionReason {
	if asOf.IsZero() || evidence.OccurredAt.IsZero() {
		return CommitmentSourceUnknown
	}
	if evidence.CancelledMeeting {
		return CommitmentMeetingCancelled
	}
	if evidence.SourceUnassessed {
		return CommitmentSourceUnassessed
	}
	if evidence.SourceIncomplete {
		return CommitmentSourceIncomplete
	}
	if !evidence.happened(asOf, zone) {
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
		if !evidence.ContextAssessed || evidence.LaterUnassessed {
			return CommitmentContextUnassessed
		}
		return CommitmentIncomplete
	}
	if evidence.OccurredAt.Before(cutoff) && (dueAt == nil || deadline.Passed(dueAt, cutoff)) {
		return CommitmentHistorical
	}
	return CommitmentCurrent
}

// happened is occurredday.Happened for the source at asOf.
func (e CommitmentAdmissionEvidence) happened(asOf time.Time, zone *time.Location) bool {
	kind := ""
	if e.Meeting {
		kind = KindMeeting
	}
	return occurredday.Happened(kind, occurredday.Moment{At: e.OccurredAt, DateOnly: e.DateOnly}, asOf, zone)
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
	query := fmt.Sprintf(`SELECT a.occurred_at, a.occurred_precision = 'day', a.raw_capture_id, a.kind = 'email' AND a.source <> 'manual', a.kind = 'meeting', coalesce(a.meeting_status = 'canceled', false),
  a.source = 'manual',
  (SELECT min(ci.source_occurred_at) FROM capture_import ci WHERE ci.activity_id = a.id),
  (SELECT min(ci.provider_received_at) FROM capture_import ci WHERE ci.activity_id = a.id),
  a.kind = 'email' AND EXISTS (SELECT 1 FROM activity later
   WHERE later.kind = a.kind AND later.thread_key = a.thread_key
    AND later.archived_at IS NULL AND `+AutomaticMailContent+`
    AND (later.occurred_at, later.id) > (a.occurred_at, a.id)
    -- A later message read only in part is later context not read.
    AND (NOT coalesce(later.id = ANY($%[1]d::uuid[]), false) OR NOT `+provenance.MailBodyCompleteSQL("later")+`)),
  a.kind = 'email' AND a.body_completeness NOT IN ('complete', 'unassessed'),
  a.kind = 'email' AND a.body_completeness = 'unassessed',
  a.kind = 'email' AND EXISTS (SELECT 1 FROM activity later
   WHERE later.kind = a.kind AND later.thread_key = a.thread_key AND later.archived_at IS NULL
    AND `+AutomaticMailContent+` AND later.body_completeness = 'unassessed'
    AND (later.occurred_at, later.id) > (a.occurred_at, a.id))
 FROM activity a WHERE a.id = $%[2]d`, arg(in.ReadMessages), arg(in.SourceID))
	if !in.Preview {
		query += " FOR UPDATE OF a"
	}
	var evidence CommitmentAdmissionEvidence
	if err := tx.QueryRow(ctx, query, args...).Scan(&evidence.OccurredAt, &evidence.DateOnly, &evidence.RawCaptureID, &evidence.CapturedMail, &evidence.Meeting, &evidence.CancelledMeeting, &evidence.LoggedByHand, &evidence.SourceOccurredAt, &evidence.ProviderReceivedAt, &evidence.UnreadLater, &evidence.SourceIncomplete, &evidence.SourceUnassessed, &evidence.LaterUnassessed); err != nil {
		return CommitmentAdmissionEvidence{}, fmt.Errorf("activities: reading commitment admission evidence: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, in.SourceID.UUID); err != nil {
		return CommitmentAdmissionEvidence{}, err
	}
	evidence.ContextAssessed = in.ReadMessages != nil
	return evidence, nil
}
