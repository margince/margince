// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deterministic evidence writers: three facts a record already states,
// turned into ledger rows without asking a model anything.
//
//   a contract turned active          → document_signed
//   a buyer confirmed a room version  → terms_accepted
//   a meeting was held with them in it → event_held
//
// Each one answers a criterion KIND rather than a named criterion: the stage
// says which of its criteria is of that kind, and a deal whose stage asks for
// nothing of the kind gets no row. That is why these take a deal and a kind
// and resolve the criterion themselves — a caller naming the criterion would
// have to know the stage's configuration, which is the thing that changes.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DeterministicClaim is one record-derived observation, before the criterion
// it settles has been resolved.
type DeterministicClaim struct {
	DealID      ids.DealID
	Kind        CriterionKind
	SourceType  string
	SourceID    ids.UUID
	SourceLines []int32
	Snippet     *string
	AuthorSide  AuthorSide
	ObservedAt  time.Time
}

// WholeSpan is the citation for evidence resting on a whole document rather
// than a passage in it: every line, 1-indexed, matching the addressing
// transcriptLineCount counts by. An empty document cites nothing.
func WholeSpan(lines int) []int32 {
	if lines <= 0 {
		return nil
	}
	span := make([]int32, lines)
	for i := range span {
		span[i] = int32(i + 1)
	}
	return span
}

// RecordDeterministicEvidence writes one record-derived claim against every
// live criterion of its kind on the deal's CURRENT stage.
//
// Answers how many rows it wrote. Zero is an ordinary outcome, not a failure:
// a stage that asks for nothing of this kind has nothing to settle, and a deal
// on such a stage is the common case rather than the exception.
//
// The claim is always CommitmentAgreed. These writers fire on records that
// already settled — a signature, a confirmation, a meeting that happened —
// and telling a proposal from an agreement is a reading task, which is the
// model's half of this ledger and not this file's.
func (s *Store) RecordDeterministicEvidence(
	ctx context.Context, claim DeterministicClaim,
) (int, error) {
	criteria, err := s.criteriaOfKind(ctx, claim.DealID, claim.Kind, claim.ObservedAt)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, criterionID := range criteria {
		if _, err := s.RecordStageEvidence(ctx, EvidenceInput{
			DealID:      claim.DealID,
			CriterionID: criterionID,
			SourceType:  claim.SourceType,
			SourceID:    claim.SourceID,
			SourceLines: claim.SourceLines,
			Snippet:     claim.Snippet,
			AuthorSide:  claim.AuthorSide,
			Commitment:  CommitmentAgreed,
			Met:         true,
			ObservedAt:  claim.ObservedAt,
			ExtractedBy: ExtractedByDeterministic,
		}); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// criteriaOfKind answers the live criteria of one kind on the stage the deal
// was on WHEN THE THING HAPPENED — not the stage it sits on now.
//
// The two differ whenever delivery lags a stage move, and the observed_at time
// is the honest anchor: a meeting held while the deal was in Discovery settles
// a Discovery criterion, whatever the deal has moved on to by the time the
// event is handled. Resolving against the current stage instead made the
// answer depend on queue latency, so the same meeting could settle different
// criteria on a replay.
//
// deal_stage_history records every move including the one at creation, so the
// stage at any past moment is the to_stage_id of the last move at or before
// that moment.
//
// A time BEFORE THE DEAL EXISTED falls back to the FIRST stage it was created
// on, rather than answering nothing. That case is ordinary rather than
// exceptional: a meeting happens, and the rep creates the deal because of it,
// so the evidence is routinely older than the record it belongs to. Refusing
// it would discard exactly the evidence that motivated the deal — and the
// first stage is where such a deal starts, so it is the honest place to hang
// it.
//
// An archived deal answers none: evidence about a deal nobody is working is a
// write with no reader.
func (s *Store) criteriaOfKind(
	ctx context.Context, dealID ids.DealID, kind CriterionKind, asOf time.Time,
) ([]ids.ExitCriterionID, error) {
	var out []ids.ExitCriterionID
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH stage_then AS (
				SELECT h.to_stage_id
				  FROM deal_stage_history h
				 WHERE h.deal_id = $1 AND h.changed_at <= $3
				 ORDER BY h.changed_at DESC, h.id DESC
				 LIMIT 1
			), stage_at_birth AS (
				SELECT h.to_stage_id
				  FROM deal_stage_history h
				 WHERE h.deal_id = $1
				 ORDER BY h.changed_at ASC, h.id ASC
				 LIMIT 1
			), stage_for_evidence AS (
				SELECT to_stage_id FROM stage_then
				 UNION ALL
				SELECT to_stage_id FROM stage_at_birth
				 WHERE NOT EXISTS (SELECT 1 FROM stage_then)
			)
			SELECT c.id
			  FROM deal d
			  JOIN stage_for_evidence s ON true
			  JOIN stage_exit_criterion c ON c.stage_id = s.to_stage_id
			 WHERE d.id = $1 AND d.archived_at IS NULL
			   AND c.archived_at IS NULL AND c.kind = $2
			 ORDER BY c."position"`, dealID, string(kind), asOf)
		if err != nil {
			return fmt.Errorf("resolve the deal's criteria of kind %s: %w", kind, err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.ExitCriterionID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("scan a criterion id: %w", err)
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out, err
}

// DealOfContract answers the deal a contract is bound to.
//
// contract.status_changed carries no deal_id — a contract belongs to an
// company and may name a deal — so the binding is read here rather than
// taken from the event. A contract naming no deal answers ErrNotFound, which
// the trigger absorbs: a company-level agreement settles no deal's
// criteria, and there is nothing to write.
func DealOfContract(ctx context.Context, tx pgx.Tx, contractID ids.UUID) (ids.DealID, error) {
	var dealID *ids.DealID
	err := tx.QueryRow(ctx,
		`SELECT deal_id FROM contract WHERE id = $1`, contractID).Scan(&dealID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.DealID{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ids.DealID{}, fmt.Errorf("read the contract's deal: %w", err)
	}
	if dealID == nil {
		return ids.DealID{}, apperrors.ErrNotFound
	}
	return *dealID, nil
}

// ActivityAuthorship answers what AuthorSideOf needs about one activity: its
// direction and its participants.
//
// A deal is named too, because an activity that reaches no deal settles
// nothing — TestAnUnlinkedActivityWritesNoEvidence is the case. activity_link
// rows are exclusive per record type, so the deal link is its own row.
type ActivityAuthorship struct {
	DealID       ids.DealID
	Direction    string
	Participants []Participant
	OccurredAt   time.Time
	// AuthorSide is who wrote it, judged against the installation's own
	// domains. It is the trail's account of who spoke; the kinds settled by a
	// recorded fact do not consult it.
	AuthorSide AuthorSide
	// Kind is the activity's own kind. Read rather than taken from the event,
	// because activity.updated carries no kind and a meeting must still be
	// told from a task.
	Kind string
	// BuyerParticipants counts the people on the activity who are genuinely
	// the other side. A meeting nobody from their side attended is one we held
	// with ourselves, and it settles no event_held criterion.
	//
	// Counted by CountsAsBuyerParticipant rather than by the absence of a
	// seat: a colleague logged through the manual path is written as a
	// person-linked row with a NULL user_id, so "not a seat" would count our
	// own people as the buyer and let an internal meeting settle the criterion.
	BuyerParticipants int
	// MeetingStatus is activity.meeting_status — booked, held, no_show or
	// canceled — and empty on anything that is not a meeting.
	MeetingStatus string
	// HasTranscript reports whether this activity carries a transcript: a
	// recording of people talking, which cannot exist for a meeting that did
	// not happen.
	HasTranscript bool
	// TranscriptLines is how many lines that transcript has, so evidence
	// written from it can cite the span it rests on.
	TranscriptLines int
}

// ReadActivityAuthorship answers what the evidence rules need to know about
// one activity — which deal it reaches, when it happened, whether it carries a
// transcript, who was on it and which side wrote it — or ErrNotFound when it
// reaches no deal.
//
// It takes the own-domain reader rather than leaving the caller to judge the
// sides, because both judgements it makes need the same list and reading it
// twice could answer differently mid-transaction. The seam stays an argument
// so this module never imports the one that owns the domains.
func ReadActivityAuthorship(
	ctx context.Context, tx pgx.Tx, activityID ids.UUID, own OwnDomainReader,
) (ActivityAuthorship, error) {
	var out ActivityAuthorship
	var direction, meetingStatus *string
	// The transcript is measured in SQL and never projected: this function needs
	// to know THAT a recording exists and how many lines it has, not what
	// anybody said in it. Selecting the body would make this a content reader of
	// a table whose audience decides who may read the text — for a judgement
	// that does not depend on a single word of it.
	//
	// The line count matches transcriptLineCount's addressing: a trailing
	// newline is punctuation rather than a final empty turn.
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, markerPos := arg(activityID), arg(TranscriptSourceSystem)
	// The timeline's own rule: an activity carries no owner, so who may read
	// one is decided by the records it links to and by its audience. This runs
	// as the system principal, where the clause is the discovery arm alone —
	// but composing it rather than waiving it is what keeps the reader correct
	// if a seat-bound caller is ever given this seam.
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return out, err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT l.deal_id, a.direction, a.occurred_at, a.meeting_status, a.kind,
		       a.source_system IS NOT DISTINCT FROM $%[2]d AND coalesce(a.body, '') <> '',
		       CASE
		         WHEN coalesce(rtrim(a.body, E'\n'), '') = '' THEN 0
		         ELSE length(rtrim(a.body, E'\n')) -
		              length(replace(rtrim(a.body, E'\n'), E'\n', '')) + 1
		       END
		  FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.deal_id IS NOT NULL
		 WHERE a.id = $%[1]d AND a.archived_at IS NULL%[3]s`,
		idPos, markerPos, scope), args...).Scan(&out.DealID, &direction,
		&out.OccurredAt, &meetingStatus, &out.Kind, &out.HasTranscript,
		&out.TranscriptLines)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read the activity's deal and direction: %w", err)
	}
	if direction != nil {
		out.Direction = *direction
	}
	if meetingStatus != nil {
		out.MeetingStatus = *meetingStatus
	}
	rows, err := tx.Query(ctx, `
		SELECT role, coalesce(user_id::text, ''), coalesce(address, ''),
		       person_id IS NOT NULL
		  FROM activity_participant WHERE activity_id = $1`, activityID)
	if err != nil {
		return out, fmt.Errorf("read the activity's participants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.Role, &p.UserID, &p.Address, &p.PersonLinked); err != nil {
			return out, fmt.Errorf("scan a participant: %w", err)
		}
		out.Participants = append(out.Participants, p)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read the activity's participants: %w", err)
	}
	domains, err := own.Domains(ctx, tx)
	if err != nil {
		return out, err
	}
	out.AuthorSide = AuthorSideOf(out.Direction, out.Participants, domains)
	for _, p := range out.Participants {
		if CountsAsBuyerParticipant(p, domains) {
			out.BuyerParticipants++
		}
	}
	return out, nil
}

// OwnDomainReader answers the email domains this installation's own people
// write from — the set a sender is tested against to tell a colleague from a
// customer.
//
// A seam rather than a query, for the reason activities.OwnDomains states: the
// domains are capture's to define, and a module may not read a sibling's
// tables. Read inside the CALLER's transaction so the authorship judgement and
// the evidence write see one snapshot.
type OwnDomainReader interface {
	Domains(ctx context.Context, tx pgx.Tx) ([]string, error)
}
