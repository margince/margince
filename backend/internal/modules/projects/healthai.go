// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The health job's own reading of a project, and a user's undo of it.
//
// The job appends to the same history a user does, through the same insert,
// marked source "ai". Two rules keep it from talking over people: a reading a
// user made stands until new activity is filed after it, and the job writes
// only a state that differs from the one standing. Both are read under the
// project lock, so a user's write racing the job is seen, not overwritten.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AIHealthOutcome says what the health job's reading did.
type AIHealthOutcome int

const (
	// AIHealthWritten appended the reading.
	AIHealthWritten AIHealthOutcome = iota + 1
	// AIHealthHeldByUser wrote nothing: a user spoke after the last
	// filed activity.
	AIHealthHeldByUser
	// AIHealthUnchanged wrote nothing: the reading would say what already
	// stands.
	AIHealthUnchanged
)

// RecordAIHealthInput is the health job's reading of one project.
type RecordAIHealthInput struct {
	ProjectID ids.ProjectID
	State     crmcontracts.ProjectHealthState
	// Note is the one-sentence reason the owner is shown.
	Note string
}

// RecordAIHealthTx appends the health job's reading inside the caller's
// transaction, unless a user's reading or the standing state says not to.
// The assessment is the zero value unless the outcome is AIHealthWritten.
func (s *Store) RecordAIHealthTx(
	ctx context.Context, tx pgx.Tx, in RecordAIHealthInput,
) (crmcontracts.ProjectHealthAssessment, AIHealthOutcome, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, 0, err
	}
	note, err := checkHealthJudgement(in.State, &in.Note)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, 0, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, 0, err
	}
	// The lock comes BEFORE the standing is read. A user's reading
	// committing between the read and the insert would otherwise be
	// overwritten by a job that never saw it.
	if err := auth.HoldWritableLive(ctx, tx, projectObject, in.ProjectID.UUID); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, 0, err
	}
	standing, err := s.readAIHealthStanding(ctx, tx, in.ProjectID)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, 0, err
	}
	if outcome := standing.decide(in.State); outcome != AIHealthWritten {
		return crmcontracts.ProjectHealthAssessment{}, outcome, nil
	}
	out, err := insertHealthRow(ctx, tx, healthRow{
		ProjectID: in.ProjectID, State: in.State, Note: note, AssessedAt: s.clock(),
		Source: sourceAI, CapturedBy: by,
	})
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, 0, err
	}
	return out, AIHealthWritten, nil
}

// aiHealthStanding is what the health job's reading is weighed against.
type aiHealthStanding struct {
	// Current is the reading that stands; nil when nobody has judged the project.
	Current *crmcontracts.ProjectHealthAssessment
	// UserSaidAt is the newest assessed_at among the rows people wrote:
	// readings, corrections and undos. Nil when no user has spoken.
	UserSaidAt *time.Time
	// ActiveAt is the instant the quiet rule measures from: the last filed
	// activity, or the project's creation.
	ActiveAt time.Time
}

// decide answers whether the health job may append a reading of proposed.
//
// An on_track reading only takes back the job's own alarm. Over a user's old
// reading, or over none, it would be a judgement the activity cannot support:
// that nothing is wrong is not something the quiet rule can see.
func (st aiHealthStanding) decide(proposed crmcontracts.ProjectHealthState) AIHealthOutcome {
	if st.UserSaidAt != nil && !st.ActiveAt.After(*st.UserSaidAt) {
		return AIHealthHeldByUser
	}
	if st.Current != nil && st.Current.State == proposed {
		return AIHealthUnchanged
	}
	byAI := st.Current != nil && st.Current.Source != nil && *st.Current.Source == sourceAI
	if proposed == crmcontracts.ProjectHealthStateOnTrack && !byAI {
		return AIHealthUnchanged
	}
	return AIHealthWritten
}

func (s *Store) readAIHealthStanding(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID,
) (aiHealthStanding, error) {
	var standing aiHealthStanding
	err := tx.QueryRow(ctx, `
		SELECT `+ProjectQuietAnchorSQL("p")+`,
		       (SELECT max(h.assessed_at) FROM project_health_assessment h
		         WHERE h.project_id = p.id AND h.source <> '`+sourceAI+`')
		FROM project p
		WHERE p.id = $1`, id).Scan(&standing.ActiveAt, &standing.UserSaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return aiHealthStanding{}, apperrors.ErrNotFound
	}
	if err != nil {
		return aiHealthStanding{}, fmt.Errorf("read project health standing: %w", err)
	}
	standing.Current, err = s.CurrentHealthTx(ctx, tx, id)
	return standing, err
}

// UndoAIHealth withdraws a reading the health job wrote.
//
// It appends an undo row that supersedes the AI reading, so the history keeps
// both. The undo row is never the current reading: what stood before the AI's
// stands again, and a project whose only reading was the AI's reads as not
// judged. The undo row carries that earlier reading's state and note because
// the table requires a state. A user wrote it, so it also holds the job off
// until new activity is filed.
func (s *Store) UndoAIHealth(
	ctx context.Context, projectID ids.ProjectID, assessmentID ids.UUID,
) (crmcontracts.ProjectHealthAssessment, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	var out crmcontracts.ProjectHealthAssessment
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, projectObject, projectID.UUID); err != nil {
			return err
		}
		target, err := lockHealthTarget(ctx, tx, projectID, assessmentID)
		if err != nil {
			return err
		}
		if target.Source != sourceAI {
			return apperrors.ErrConflict
		}
		row := healthRow{
			ProjectID: projectID, State: crmcontracts.ProjectHealthStateOnTrack,
			AssessedAt: target.AssessedAt, Source: sourceUndo, CapturedBy: by,
			Supersedes: &assessmentID,
		}
		if err := restoreEarlierReading(ctx, tx, projectID, assessmentID, &row); err != nil {
			return err
		}
		out, err = insertHealthRow(ctx, tx, row)
		return err
	})
	return out, err
}

// restoreEarlierReading copies onto the undo row the state and note of the
// reading that stood before the AI's: the newest one before it in the
// history's own order that nothing superseded. With none, the row keeps the
// on_track it was given.
func restoreEarlierReading(
	ctx context.Context, tx pgx.Tx, projectID ids.ProjectID, aiReading ids.UUID, row *healthRow,
) error {
	var state string
	var note *string
	err := tx.QueryRow(ctx, `
		SELECT a.state, a.note
		FROM project_health_assessment a
		JOIN project_health_assessment t ON t.id = $2 AND t.project_id = a.project_id
		WHERE a.project_id = $1
		  AND a.source <> '`+sourceUndo+`'
		  AND (a.assessed_at, a.created_at, a.id) < (t.assessed_at, t.created_at, t.id)
		  AND NOT EXISTS (SELECT 1 FROM project_health_assessment s WHERE s.supersedes_assessment_id = a.id)
		ORDER BY a.assessed_at DESC, a.created_at DESC, a.id DESC
		LIMIT 1`, projectID, aiReading).Scan(&state, &note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the reading before the AI's: %w", err)
	}
	row.State, row.Note = crmcontracts.ProjectHealthState(state), note
	return nil
}
