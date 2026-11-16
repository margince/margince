// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The preview's advertising content edge, bound to the consent module, which
// owns the packs and reads each recipient's country.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
)

type draftContentAdapter struct {
	store *consent.Store
}

// CheckAdvertisingDraft answers what the draft does not meet.
func (a draftContentAdapter) CheckAdvertisingDraft(
	ctx context.Context, recipients []string, subject, body string,
) ([]activities.DraftContentFinding, error) {
	found, err := a.store.CheckAdvertisingDraft(ctx, recipients, subject, body)
	if err != nil {
		return nil, err
	}
	out := make([]activities.DraftContentFinding, 0, len(found))
	for _, f := range found {
		out = append(out, activities.DraftContentFinding{
			Requirement: f.Requirement, Detail: f.Detail, Words: f.Words, Language: f.Language,
		})
	}
	return out, nil
}
