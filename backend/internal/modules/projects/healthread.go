// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Reading how a project has been going, and what that means now.
//
// The CURRENT reading is derived rather than stored, which is the whole reason
// the table is append-only. One SQL fragment answers it, shared by the list
// projection and the project page, so the two cannot disagree about which
// judgement stands.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sourceHuman is what a person acting in the UI writes. Connectors and imports
// would name themselves; nothing but people judge a project today.
const sourceHuman = "human"

// healthColumns is what a health read returns, shared by the list and the
// single read.
const healthColumns = `a.id, a.project_id, a.state, a.note, a.assessed_at,
	a.source, a.source_author, a.created_at, a.supersedes_assessment_id,
	(SELECT s.id FROM project_health_assessment s WHERE s.supersedes_assessment_id = a.id)`

// currentHealthWhere narrows to the judgement that STANDS for a project: the
// newest one nothing has corrected.
//
// Superseded rows are excluded rather than ordered around. A correction is the
// statement that its target was wrong, so a corrected reading is not a stale
// answer to be outranked — it is not an answer at all.
const currentHealthWhere = `a.project_id = $%d
	AND NOT EXISTS (SELECT 1 FROM project_health_assessment s WHERE s.supersedes_assessment_id = a.id)
	ORDER BY a.assessed_at DESC, a.created_at DESC, a.id DESC
	LIMIT 1`

func scanHealth(sc interface{ Scan(...any) error }) (crmcontracts.ProjectHealthAssessment, error) {
	var out crmcontracts.ProjectHealthAssessment
	var id, projectID ids.UUID
	var state string
	var note, source, sourceAuthor *string
	var assessedAt, createdAt time.Time
	var supersedes, supersededBy *ids.UUID
	if err := sc.Scan(&id, &projectID, &state, &note, &assessedAt,
		&source, &sourceAuthor, &createdAt, &supersedes, &supersededBy); err != nil {
		return crmcontracts.ProjectHealthAssessment{}, err
	}
	superseded := supersededBy != nil
	out = crmcontracts.ProjectHealthAssessment{
		Id:           openapi_types.UUID(id),
		ProjectId:    openapi_types.UUID(projectID),
		State:        crmcontracts.ProjectHealthState(state),
		Note:         note,
		AssessedAt:   assessedAt,
		Source:       source,
		SourceAuthor: sourceAuthor,
		CreatedAt:    &createdAt,
		// A corrected reading says so, so a reader of the history knows which
		// line was withdrawn rather than seeing two judgements for one day.
		Superseded: &superseded,
	}
	if supersededBy != nil {
		converted := openapi_types.UUID(*supersededBy)
		out.SupersededById = &converted
	}
	if supersedes != nil {
		converted := openapi_types.UUID(*supersedes)
		out.SupersedesAssessmentId = &converted
	}
	return out, nil
}

func readHealthAssessment(
	ctx context.Context, tx pgx.Tx, id ids.UUID,
) (crmcontracts.ProjectHealthAssessment, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+healthColumns+` FROM project_health_assessment a WHERE a.id = $1`, id)
	out, err := scanHealth(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.ProjectHealthAssessment{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.ProjectHealthAssessment{}, fmt.Errorf("read project health assessment: %w", err)
	}
	return out, nil
}

// CurrentHealthTx answers the judgement that stands for one project, inside a
// caller-opened transaction. Absent means nobody has judged it — which is a
// different answer from "judged and found healthy", and the page says so.
//
// Gated on project read like every other entry point, even though both callers
// already hold it: the cost is one map lookup, and a seam that trusts its
// callers is one refactor away from being reached by one that does not.
func (s *Store) CurrentHealthTx(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID,
) (*crmcontracts.ProjectHealthAssessment, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	row := tx.QueryRow(ctx, storekit.SQLf(
		`SELECT `+healthColumns+` FROM project_health_assessment a WHERE `+currentHealthWhere,
		arg(id)), args...)
	out, err := scanHealth(row)
	if errors.Is(err, pgx.ErrNoRows) {
		//nolint:nilnil // "nobody has judged this project" IS the answer, not a
		// missing one — and it is a different answer from "judged and found
		// healthy", which is why the page renders the two differently.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read current project health: %w", err)
	}
	return &out, nil
}

// ListHealth reads a project's judgements newest first, corrections included.
//
// The whole history, because that is what a delivery review asks for: what was
// said in March, not only what is true now. A corrected reading stays in the
// list and is marked, rather than disappearing and leaving a gap nobody can
// account for.
func (s *Store) ListHealth(
	ctx context.Context, id ids.ProjectID, limitIn *int,
) ([]crmcontracts.ProjectHealthAssessment, storekit.Page, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	var out []crmcontracts.ProjectHealthAssessment
	var info storekit.Page
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// The history is a fact about the record, gated exactly as the record
		// is: a caller who cannot see the project is told it is not there.
		if err := auth.EnsureVisible(ctx, tx, projectObject, id.UUID); err != nil {
			return err
		}
		limit := 50
		if limitIn != nil && *limitIn > 0 && *limitIn <= 200 {
			limit = *limitIn
		}
		rows, err := tx.Query(ctx, `
			SELECT `+healthColumns+`
			FROM project_health_assessment a
			WHERE a.project_id = $1
			ORDER BY a.assessed_at DESC, a.created_at DESC, a.id DESC
			LIMIT $2`, id, limit+1)
		if err != nil {
			return fmt.Errorf("list project health assessments: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			assessment, err := scanHealth(rows)
			if err != nil {
				return fmt.Errorf("scan project health assessment: %w", err)
			}
			out = append(out, assessment)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) > limit {
			out = out[:limit]
			info.HasMore = true
		}
		return nil
	})
	return out, info, err
}
