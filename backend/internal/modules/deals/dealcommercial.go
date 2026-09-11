// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal's commercial context — the brief a person wrote, why the deal
// exists, how much a human says it matters, and which channel brought it.
//
// Its own file rather than more of deal.go: those four columns are read and
// patched together, and the enum conversions below are needed at every one of
// those call sites.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// motionOf and priorityOf read the deal's current commercial enums as the
// plain strings a patch's before-image compares. The contract spells each as
// its own string type, so the conversion is needed at every call site; doing
// it here keeps the patch readable and the nil case in one place.
func motionOf(d crmcontracts.Deal) *string {
	if d.CommercialMotion == nil {
		return nil
	}
	s := string(*d.CommercialMotion)
	return &s
}

func priorityOf(d crmcontracts.Deal) *string {
	if d.Priority == nil {
		return nil
	}
	s := string(*d.Priority)
	return &s
}

// appendCommercialFilters narrows a deal list by the three commercial-context
// columns.
//
// Each also admits the sentinel "unset", which asks for the deals carrying no
// value at all — a question no enum member can pose. Spelled as a sentinel
// rather than an empty parameter because an empty string is what a cleared
// form field sends, and "show me everything" is what THAT caller meant.
func appendCommercialFilters(where []string, in ListDealsInput, arg func(any) int) []string {
	for _, f := range []struct {
		column string
		value  *string
	}{
		{filterCommercialMotion, in.CommercialMotion},
		{filterPriority, in.Priority},
		{filterAcquisitionSource, in.AcquisitionSource},
	} {
		if f.value == nil {
			continue
		}
		if *f.value == filterUnset {
			where = append(where, storekit.SQLf("%s IS NULL", f.column))
			continue
		}
		where = append(where, storekit.SQLf("%s = $%d", f.column, arg(*f.value)))
	}
	return where
}
