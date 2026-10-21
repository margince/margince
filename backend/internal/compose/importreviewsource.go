// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type importReviewSource struct {
	objects      []string
	rows         map[string][]migration.Row
	associations []migration.Assoc
}

func importSourceFromReview(plan migration.ReviewPlan, mapping migration.RunMapping) (importReviewSource, error) {
	out := importReviewSource{rows: map[string][]migration.Row{}, associations: plan.Associations}
	for _, object := range migration.ImportOrder {
		for _, part := range mapping.PartList() {
			if part.Object == object {
				out.objects = append(out.objects, object)
			}
		}
	}
	for _, reviewed := range plan.Rows {
		if !slices.Contains(out.objects, reviewed.Object) {
			return importReviewSource{}, apperrors.ErrInvalidArgument
		}
		raw, err := json.Marshal(reviewed.Values)
		if err != nil {
			return importReviewSource{}, err
		}
		row := migration.Row{ExternalID: reviewed.ExternalID, Line: reviewed.Line, OwnerExternalID: reviewed.OwnerExternalID, LastSyncedAt: reviewed.LastSyncedAt}
		if err := json.Unmarshal(raw, &row.Fields); err != nil {
			return importReviewSource{}, err
		}
		out.rows[reviewed.Object] = append(out.rows[reviewed.Object], row)
	}
	return out, nil
}

func (s importReviewSource) Objects() []string { return slices.Clone(s.objects) }

func (s importReviewSource) Counts(context.Context) (map[string]int, error) {
	out := make(map[string]int, len(s.rows))
	for kind, rows := range s.rows {
		out[kind] = len(rows)
	}
	return out, nil
}

func (s importReviewSource) Rows(_ context.Context, object string, offset, limit int) ([]migration.Row, error) {
	if offset < 0 || limit < 0 {
		return nil, apperrors.ErrInvalidArgument
	}
	rows := s.rows[object]
	if offset >= len(rows) {
		return []migration.Row{}, nil
	}
	return slices.Clone(rows[offset : offset+min(len(rows)-offset, limit)]), nil
}

func (s importReviewSource) Associations(context.Context) ([]migration.Assoc, error) {
	return slices.Clone(s.associations), nil
}
