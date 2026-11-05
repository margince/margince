// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MailBodyAssessment is how much of one unassessed mail's stored body holds
// its text, decided from that body as it was read.
type MailBodyAssessment struct {
	Activity     ids.UUID
	Completeness string
	Body         string
}

// AssessMailBodiesTx records the assessments of mail still unassessed. A row
// whose body changed since it was read keeps its state: the assessment was of
// the old text. Only the completeness column is written; the body, the files
// and every time stay as they are.
func AssessMailBodiesTx(ctx context.Context, tx pgx.Tx, assessments []MailBodyAssessment) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	changed := 0
	for _, assessment := range assessments {
		tag, err := tx.Exec(ctx, `UPDATE activity SET body_completeness = $2
			WHERE id = $1 AND body_completeness = 'unassessed' AND body = $3`,
			assessment.Activity, assessment.Completeness, assessment.Body)
		if err != nil {
			return changed, fmt.Errorf("activities: recording a mail body assessment: %w", err)
		}
		changed += int(tag.RowsAffected())
	}
	return changed, nil
}
