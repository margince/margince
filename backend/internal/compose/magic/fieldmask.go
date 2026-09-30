// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// What a receipt may show of a change: the fields the reader could read on the
// record itself, and nothing more.
//
// Both surfaces read straight from the audit images, which hold every field the
// write touched. A role that withholds a deal's amount on the deal page must
// not find it here, one click away from the receipt — nor on the page line,
// which carries the same two images in full.

import (
	"context"
	"maps"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// referenceFields are links to OTHER records. The record's own read withholds
// each one the reader cannot open; a receipt would show the raw id either way,
// which names nothing a reader can use and discloses that the record exists.
// Left out for everyone.
var referenceFields = map[string]bool{
	"company_id": true, "project_id": true, "partner_company_id": true,
	"partner_attribution": true, "merged_into_id": true,
}

// imageMask answers what one reader may not see, per record type. Built once
// per read rather than per line: the configured masks do not move inside a
// request, and a page draws a line per audit row.
type imageMask struct {
	actor  principal.Principal
	byType map[string]map[string]bool
}

func newImageMask(ctx context.Context) (imageMask, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return imageMask{}, err
	}
	return imageMask{actor: actor, byType: map[string]map[string]bool{}}, nil
}

// hidden is the strictest reading: a mask that lifts for a record the caller
// may write is kept, because neither receipt read asks that question per row.
func (m imageMask) hidden(entityType string) map[string]bool {
	if known, computed := m.byType[entityType]; computed {
		return known
	}
	// auth.MaskedFields already widens a configured name to everything
	// withheld with it, so the money group arrives whole and this surface has
	// no second opinion about what travels together.
	hidden := map[string]bool{}
	for _, field := range auth.MaskedFields(m.actor, entityType, false) {
		hidden[field] = true
	}
	maps.Copy(hidden, referenceFields)
	m.byType[entityType] = hidden
	return hidden
}

// withheldFrom answers the image without the fields this reader may not see.
// A nil image stays nil: the row recorded no such side, which is not the same
// as one masked down to nothing.
func (m imageMask) withheldFrom(entityType string, image *map[string]any) *map[string]any {
	if image == nil {
		return nil
	}
	hidden := m.hidden(entityType)
	shown := map[string]any{}
	for field, value := range *image {
		if !hidden[field] {
			shown[field] = value
		}
	}
	return &shown
}

// visibleChanges drops the fields the caller's role masks on this record type.
func visibleChanges(ctx context.Context, entityType string, changes []crmcontracts.MagicFieldChange) ([]crmcontracts.MagicFieldChange, error) {
	mask, err := newImageMask(ctx)
	if err != nil {
		return nil, err
	}
	hidden := mask.hidden(entityType)
	out := make([]crmcontracts.MagicFieldChange, 0, len(changes))
	for _, c := range changes {
		if hidden[c.Field] {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
