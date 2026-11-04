// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// How a project is going, as somebody judged it on a day.
//
// Append-only. A `health` column on project would answer "how is it now" and
// destroy the answer to "how was it in March", which is the question a delivery
// review actually asks. Every assessment stays and the current one is derived.
//
// A mistake is corrected by SUPERSEDING the row, never by editing it: the
// successor carries the target's effective time, so a correction fixes what was
// said without moving when it was said.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	healthTable       = "project_health_assessment"
	healthObject      = "project_health_assessment"
	healthStateColumn = "state"
	healthNoteColumn  = "note"
	healthMaxNote     = 4000
)

// RecordHealthInput is an appended judgement.
type RecordHealthInput struct {
	ProjectID  ids.ProjectID
	State      crmcontracts.ProjectHealthState
	Note       *string
	AssessedAt *time.Time
	// SourceAuthor names who judged it when that is not the caller — a lead
	// entering what a delivery manager said keeps them on the record while
	// captured_by stays the authenticated session.
	SourceAuthor *string
}

// CorrectHealthInput replaces one reading. There is no AssessedAt: a correction
// keeps the target's, which the server reads from the row being corrected.
type CorrectHealthInput struct {
	ProjectID    ids.ProjectID
	AssessmentID ids.UUID
	State        crmcontracts.ProjectHealthState
	Note         *string
	SourceAuthor *string
}

// RecordHealth appends a judgement about how a project is going.
func (s *Store) RecordHealth(
	ctx context.Context, in RecordHealthInput,
) (crmcontracts.ProjectHealthAssessment, error) {
	var out crmcontracts.ProjectHealthAssessment
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.RecordHealthTx(ctx, tx, in)
		return err
	})
	return out, err
}

// RecordHealthTx is RecordHealth inside a caller-opened transaction.
func (s *Store) RecordHealthTx(
	ctx context.Context, tx pgx.Tx, in RecordHealthInput,
) (crmcontracts.ProjectHealthAssessment, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	note, err := checkHealthJudgement(in.State, in.Note)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	assessedAt := s.clock()
	if in.AssessedAt != nil {
		assessedAt = *in.AssessedAt
	}
	// A reading nobody could have taken yet would become current the moment it
	// landed and stay current until the clock caught up, hiding every real
	// judgement made in between.
	if assessedAt.After(s.clock()) {
		return crmcontracts.ProjectHealthAssessment{}, &values.ParseError{
			Field: "assessed_at", Code: "future_assessment",
			Message: "a judgement cannot be dated in the future",
		}
	}
	// The project is HELD, not merely probed: an archive committing between
	// the check and the insert would file a judgement on a closed record,
	// and the foreign key would not notice because archiving does not
	// delete the row. Holding the parent first is also the order Art. 17
	// erasure takes, so this writer never deadlocks against it.
	if err := auth.HoldWritableLive(ctx, tx, projectObject, in.ProjectID.UUID); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	return insertHealthRow(ctx, tx, healthRow{
		ProjectID: in.ProjectID, State: in.State, Note: note, AssessedAt: assessedAt,
		Source: sourceHuman, SourceAuthor: trimmedOrNil(in.SourceAuthor), CapturedBy: by,
	})
}

// CorrectHealth replaces one reading with a corrected one, keeping when it was
// said.
func (s *Store) CorrectHealth(
	ctx context.Context, in CorrectHealthInput,
) (crmcontracts.ProjectHealthAssessment, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	note, err := checkHealthJudgement(in.State, in.Note)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	var out crmcontracts.ProjectHealthAssessment
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, projectObject, in.ProjectID.UUID); err != nil {
			return err
		}
		target, err := lockHealthTarget(ctx, tx, in.ProjectID, in.AssessmentID)
		if err != nil {
			return err
		}
		// An undo row withdraws the AI reading it replaced and counts for
		// nothing itself, so it holds no judgement to correct.
		if target.Source == sourceUndo {
			return apperrors.ErrConflict
		}
		out, err = insertHealthRow(ctx, tx, healthRow{
			ProjectID: in.ProjectID, State: in.State, Note: note, AssessedAt: target.AssessedAt,
			Source: sourceHuman, SourceAuthor: trimmedOrNil(in.SourceAuthor), CapturedBy: by,
			Supersedes: &in.AssessmentID,
		})
		return err
	})
	return out, err
}

// healthTarget is the reading a correction or an undo replaces.
type healthTarget struct {
	AssessedAt time.Time
	Source     string
}

// lockHealthTarget locks the reading a successor will replace, and refuses one
// that already has a successor.
//
// FOR UPDATE keeps the chain linear. Read without it, two colleagues
// correcting the same reading both find it uncorrected, both insert a
// successor, and the unique index refuses the loser at commit with nothing to
// say why. The lock makes them take turns, so the second is told the row is
// already corrected.
//
// The project id is in the WHERE clause, so a successor naming another
// project's reading finds nothing here rather than relying on the composite
// foreign key to catch it later.
func lockHealthTarget(
	ctx context.Context, tx pgx.Tx, projectID ids.ProjectID, assessmentID ids.UUID,
) (healthTarget, error) {
	var target healthTarget
	var supersededBy *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT a.assessed_at, a.source,
		       (SELECT s.id FROM project_health_assessment s
		         WHERE s.supersedes_assessment_id = a.id)
		FROM project_health_assessment a
		WHERE a.id = $1 AND a.project_id = $2
		FOR UPDATE OF a`,
		assessmentID, projectID).Scan(&target.AssessedAt, &target.Source, &supersededBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return healthTarget{}, apperrors.ErrNotFound
	}
	if err != nil {
		return healthTarget{}, fmt.Errorf("lock project health assessment: %w", err)
	}
	if supersededBy != nil {
		// A second successor would fork the chain. Replace the reading that
		// stands instead.
		return healthTarget{}, apperrors.ErrConflict
	}
	return target, nil
}

// healthRow is one row of the history, as every writer appends it.
type healthRow struct {
	ProjectID    ids.ProjectID
	State        crmcontracts.ProjectHealthState
	Note         *string
	AssessedAt   time.Time
	Source       string
	SourceAuthor *string
	CapturedBy   string
	Supersedes   *ids.UUID
}

// insertHealthRow is the one INSERT into project_health_assessment. A user's
// reading, a correction, the health job's reading and an undo all append
// through it, so each of them writes its audit row and its event the same way.
func insertHealthRow(
	ctx context.Context, tx pgx.Tx, row healthRow,
) (crmcontracts.ProjectHealthAssessment, error) {
	id := ids.NewV7()
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := storekit.SQLf(`
		INSERT INTO project_health_assessment
		  (id, project_id, state, note, assessed_at, source, source_author, captured_by,
		   supersedes_assessment_id)
		VALUES ($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)`,
		arg(id), arg(row.ProjectID), arg(string(row.State)), arg(row.Note), arg(row.AssessedAt),
		arg(row.Source), arg(row.SourceAuthor), arg(row.CapturedBy), arg(row.Supersedes))
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		if storekit.IsUniqueViolation(err) {
			return crmcontracts.ProjectHealthAssessment{}, apperrors.ErrConflict
		}
		return crmcontracts.ProjectHealthAssessment{}, fmt.Errorf("insert project health assessment: %w", err)
	}
	after := map[string]any{
		"project_id": row.ProjectID.String(), healthStateColumn: string(row.State), "source": row.Source,
	}
	if row.Supersedes != nil {
		after["corrects"] = row.Supersedes.String()
	}
	auditID, err := storekit.Audit(ctx, tx, "create", healthObject, id, nil, after)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, row.ProjectID.UUID, crmcontracts.PublicEventProjectHealthAssessed{
		AssessmentId: openapi_types.UUID(id), State: string(row.State), Source: row.Source,
	}); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, fmt.Errorf("emit project.health_assessed: %w", err)
	}
	return readHealthAssessment(ctx, tx, id)
}

// checkHealthJudgement holds the rules a judgement must satisfy, applied by
// the append, the correction and the health job.
func checkHealthJudgement(
	state crmcontracts.ProjectHealthState, note *string,
) (*string, error) {
	if !state.Valid() {
		return nil, &values.ParseError{
			Field: healthStateColumn, Code: "invalid_state",
			Message: "state is one of on_track, at_risk, off_track",
		}
	}
	trimmed := trimmedOrNil(note)
	// RUNES, not bytes. The contract's maxLength and the SQL length() both count
	// characters, so counting bytes here refuses a note those two accept — a
	// 3,000-character German or Vietnamese note is well inside both limits and
	// over 4,000 bytes.
	if trimmed != nil && utf8.RuneCountInString(*trimmed) > healthMaxNote {
		return nil, &values.ParseError{
			Field: healthNoteColumn, Code: "note_too_long",
			Message: fmt.Sprintf("a note is at most %d characters", healthMaxNote),
		}
	}
	// Anything but on_track owes a reason: a risk nobody explained is an alarm
	// nobody can act on, and whoever could say why is the one filing it.
	if state != crmcontracts.ProjectHealthStateOnTrack && trimmed == nil {
		return nil, &values.ParseError{
			Field: healthNoteColumn, Code: "note_required",
			Message: "say what is wrong: a note is required unless the project is on track",
		}
	}
	return trimmed, nil
}

func trimmedOrNil(in *string) *string {
	if in == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*in)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
