// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// What an opened line may show of a change: the fields the reader could read on
// the record itself, and nothing more.
//
// The change list is read straight from the audit images, which hold every
// field the write touched. A role that withholds a deal's amount on the deal
// page must not find it here, one click away from the receipt.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// coupledMasks are fields withheld together, the way the record's own read
// withholds them: a currency or an ARR beside a withheld amount still says how
// big the deal is (deals/fieldmask.go).
var coupledMasks = map[string][]string{
	"amount_minor":       dealMoneyFields,
	"expected_arr_minor": dealMoneyFields,
}

// dealMoneyFields are the three a deal's own read withholds as one.
var dealMoneyFields = []string{"amount_minor", "expected_arr_minor", "currency"}

// referenceFields are links to OTHER records. The record's own read withholds
// each one the reader cannot open; this list would show the raw id either way,
// which names nothing a reader can use and discloses that the record exists.
// Left out of the change list for everyone.
var referenceFields = map[string]bool{
	"company_id": true, "project_id": true, "partner_company_id": true,
	"partner_attribution": true, "merged_into_id": true,
}

// visibleChanges drops the fields the caller's role masks on this record type,
// at the strictest reading: a mask that lifts for a record the caller may
// write is kept here, because this read does not ask that question per row.
func visibleChanges(ctx context.Context, entityType string, changes []crmcontracts.MagicFieldChange) ([]crmcontracts.MagicFieldChange, error) {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	for _, field := range auth.MaskedFields(p, entityType, false) {
		hidden[field] = true
		for _, coupled := range coupledMasks[field] {
			hidden[coupled] = true
		}
	}
	out := make([]crmcontracts.MagicFieldChange, 0, len(changes))
	for _, c := range changes {
		if hidden[c.Field] || referenceFields[c.Field] {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
