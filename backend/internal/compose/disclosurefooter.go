// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The send path's disclosure edge, bound to the consent module.
//
// activities composes a message body and must not know what a jurisdiction pack
// is; consent owns the packs, the installation's own particulars, and the
// mapping from an obligation to the words that meet it. Neither imports the
// other, so the edge is injected here like every other cross-module edge.
//
// This is the caller gates/messagingruleapplied_test.go named as missing: the
// disclosures were declared, rendered and put nowhere.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type disclosureAdapter struct {
	store *consent.Store
}

// DisclosuresFor answers one line per obligation the category carries.
//
// The CATEGORY crosses the seam as a string. activities does import commsauthz
// — the shared vocabulary is a port, not a sibling module — but the seam is
// declared by the consumer and a plain string keeps the interface free of a
// type the resolver owns the meaning of.
//
// An unknown string is not an error. It resolves to a category no pack marks
// marketing-only, so the message carries the obligations binding every first
// contact and none of the advertising-only ones — the safe direction, because
// putting an objection route under what may be an invoice reads as an
// invitation to stop receiving invoices.
func (a disclosureAdapter) DisclosuresFor(
	ctx context.Context, category string,
) ([]activities.DisclosureLine, error) {
	owed, err := a.store.Disclosures(ctx, commsauthz.Category(category))
	if err != nil {
		return nil, err
	}
	out := make([]activities.DisclosureLine, 0, len(owed))
	for _, line := range owed {
		out = append(out, activities.DisclosureLine{Kind: line.Kind, Text: line.Text})
	}
	return out, nil
}
