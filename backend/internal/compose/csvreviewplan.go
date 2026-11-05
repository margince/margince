// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (w *csvWriters) markPredictedRow(row migration.Row, outcome predictedOutcome) {
	if outcome == predictUnwritable || outcome == predictCollidesSkipped {
		w.links.markRefused(w.object, row.ExternalID)
		return
	}
	w.links.markPending(w.object, row.ExternalID)
	w.links.markNaturalPending(w.object, textFields(row.Fields)[naturalKeyField(w.object)])
	if w.object == migration.ObjectCompany {
		fields := textFields(row.Fields)
		creates := outcome == predictCreate || outcome == predictCollides
		w.links.markNamesPending(creates, fields[fieldDisplayName], fields[fieldLegalName])
	}
	if w.object == migration.ObjectContact {
		w.links.markEmployerPending(w.predictedEmployers[row.ExternalID])
	}
}

func reviewDisposition(outcome predictedOutcome) (string, error) {
	switch outcome {
	case predictCreate:
		return "create", nil
	case predictCollides:
		return "create_duplicate", nil
	case predictUpdate:
		return "update", nil
	case predictUnchanged:
		return "unchanged", nil
	case predictUnwritable, predictCollidesSkipped:
		return importRowSkip, nil
	default:
		return "", apperrors.ErrInvalidArgument
	}
}

func (w *csvWriters) reviewRow(ctx context.Context, row migration.Row, outcome predictedOutcome) (migration.ReviewRow, error) {
	out := migration.ReviewRow{Object: w.object, ExternalID: row.ExternalID, Line: row.Line, OwnerExternalID: row.OwnerExternalID, LastSyncedAt: row.LastSyncedAt}
	var err error
	out.Outcome, err = reviewDisposition(outcome)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(row.Fields)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out.Values); err != nil {
		return out, err
	}
	if out.Outcome != importRowSkip {
		if err := w.reviewRowValues(ctx, row, &out); err != nil {
			return out, err
		}
	}
	if outcome != predictUpdate && outcome != predictUnchanged {
		return out, nil
	}
	target, err := w.reviewTarget(ctx, row)
	if err != nil {
		return out, err
	}
	current, err := w.read(ctx, target)
	if err != nil {
		return out, err
	}
	var record struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(current, &record); err != nil {
		return out, err
	}
	if target.IsZero() || record.Version <= 0 {
		return out, apperrors.ErrInvalidArgument
	}
	out.TargetID, out.TargetVersion = target.String(), record.Version
	return out, nil
}

func (w *csvWriters) reviewCheckedCells(ctx context.Context, row migration.Row) (map[string]any, error) {
	typed, invalid, _ := w.landingCells(row)
	if invalid != nil {
		return nil, apperrors.ErrVersionSkew
	}
	author := authorFrom(textFields(row.Fields))
	if w.object == migration.ObjectDeal || author.AuthorName == nil || !strings.Contains(*author.AuthorName, "@") {
		return typed, nil
	}
	if err := w.resolveSourceAuthor(ctx, &author); err != nil {
		return nil, err
	}
	checked := make(map[string]any, len(typed)+1)
	maps.Copy(checked, typed)
	checked[previewFieldSourceAuthorID] = author.AuthorID
	return checked, nil
}

func (w *csvWriters) reviewTarget(ctx context.Context, row migration.Row) (ids.UUID, error) {
	target := w.targetIDOf(ctx, row)
	if target.named {
		if target.why.Code != "" {
			return ids.UUID{}, apperrors.ErrInvalidArgument
		}
		return target.id, nil
	}
	id, found, err := w.lookup(ctx, w.object, row.ExternalID)
	if err != nil {
		return ids.UUID{}, err
	}
	if !found {
		return ids.UUID{}, apperrors.ErrNotFound
	}
	return id, nil
}

func (w *csvWriters) reviewRowValues(ctx context.Context, row migration.Row, out *migration.ReviewRow) error {
	typed, err := w.reviewCheckedCells(ctx, row)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(typed)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &out.CheckedValues); err != nil {
		return err
	}
	out.Links, err = w.reviewLinks(ctx, row)
	if err != nil {
		return err
	}
	return nil
}
