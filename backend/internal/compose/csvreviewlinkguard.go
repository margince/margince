// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func settleReviewLinks(plan *migration.ReviewPlan) {
	for i := range plan.Rows {
		for j := range plan.Rows[i].Links {
			link := &plan.Rows[i].Links[j]
			if !link.Pending {
				continue
			}
			var match *migration.ReviewRow
			for k := range plan.Rows {
				row := &plan.Rows[k]
				if row.Object != link.Object || row.Outcome == importRowSkip || !reviewLinkNamesRow(*link, *row) {
					continue
				}
				if match != nil {
					plan.Complete = false
					return
				}
				match = row
			}
			if match == nil {
				plan.Complete = false
				return
			}
			link.PendingKey = match.ExternalID
			if match.TargetID != "" {
				link.TargetID, link.Pending = match.TargetID, false
			}
		}
	}
}

func reviewLinkNamesRow(link migration.ReviewLink, row migration.ReviewRow) bool {
	if !link.ByName && link.Key == row.ExternalID {
		return true
	}
	if link.ByName {
		return employerKey(link.Key) == employerKey(reviewText(row.Values[fieldDisplayName])) ||
			employerKey(link.Key) == employerKey(reviewText(row.Values[fieldLegalName]))
	}
	wanted, valid := naturalKeyFor(link.Object, link.Key)
	if !valid {
		return false
	}
	actual, valid := naturalKeyFor(row.Object, reviewText(row.Values[naturalKeyField(row.Object)]))
	return valid && actual == wanted
}

func reviewText(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func (w *csvWriters) validateReviewedLinks(ctx context.Context, external string) error {
	if w.reviewRows == nil {
		return nil
	}
	row, found := w.reviewRows[external]
	if !found {
		return apperrors.ErrVersionSkew
	}
	for _, expected := range row.Links {
		current, err := w.resolveReviewLink(ctx, expected)
		if err != nil {
			return err
		}
		if expected.Pending {
			binding, found, err := w.identities.LookupBinding(ctx, w.sourceSystem, expected.Object, expected.PendingKey)
			if err != nil {
				return err
			}
			if !found || binding.RunID != w.runID || current.TargetID != binding.NativeID.String() {
				return apperrors.ErrVersionSkew
			}
		} else if expected.Object == migration.ObjectCompany && expected.TargetID == "" && !expected.Dropped && current.TargetID != "" {
			binding, found, err := w.identities.LookupBinding(ctx, w.sourceSystem, migration.ObjectCompany, employerImportKey(current.TargetID))
			if err != nil {
				return err
			}
			if !found || binding.RunID != w.runID {
				return apperrors.ErrVersionSkew
			}
		} else if current.TargetID != expected.TargetID || current.Dropped != expected.Dropped || current.Pending {
			return apperrors.ErrVersionSkew
		}
	}
	return nil
}
