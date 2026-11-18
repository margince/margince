// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal's commercial context — the brief a colleague wrote, why the deal
// exists, how much a human says it matters, and which channel brought it.
//
// Its own file rather than more of deal.go: those four columns are read and
// patched together, and the enum conversions below are needed at every one of
// those call sites.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// CommercialFilterPredicates are the three commercial-context filters, keyed by
// wire name, each a template with one %[1]s for the bound value. The deal list
// and the deals-by-stage report both render them, so the two count one set.
//
// Each also admits the sentinel "unset", which asks for the deals carrying no
// value at all — a question no enum member can pose. Spelled as a sentinel
// rather than an empty parameter because an empty string is what a cleared
// form field sends, and "show me everything" is what THAT caller meant.
func CommercialFilterPredicates(alias string) map[string]string {
	out := make(map[string]string, len(commercialFilters))
	for _, name := range commercialFilters {
		out[name] = fmt.Sprintf("(%[1]s = %%[1]s OR (%[1]s IS NULL AND %%[1]s = '%[2]s'))",
			columnPrefix(alias)+name, filterUnset)
	}
	return out
}

// commercialFilters are wire names that are also the deal's column names.
var commercialFilters = []string{filterCommercialMotion, filterPriority, filterAcquisitionSource}

// appendCommercialFilters narrows a deal list by the three commercial-context
// columns and by link type.
func appendCommercialFilters(where []string, in ListDealsInput, arg func(any) int) []string {
	predicates := CommercialFilterPredicates("")
	values := map[string]*string{
		filterCommercialMotion:  in.CommercialMotion,
		filterPriority:          in.Priority,
		filterAcquisitionSource: in.AcquisitionSource,
	}
	for _, name := range commercialFilters {
		if value := values[name]; value != nil {
			where = append(where, storekit.SQLf(predicates[name], storekit.SQLf("$%d", arg(*value))))
		}
	}
	if in.LinkType != nil {
		where = append(where, storekit.DealLinkFilter("deal.id", storekit.SQLf("$%d", arg(*in.LinkType))))
	}
	return where
}

// lockedAcquisitionSource takes the deal's row lock and re-reads the source
// under it, so the "already holds this key" exemption is decided on a value
// that cannot move before the write.
//
// The unlocked read the caller already did is fine for an audit before-image;
// it is not fine for a decision about what may be assigned. Deal first, then
// the catalog row, which is the order every writer in this package takes.
func lockedAcquisitionSource(
	ctx context.Context, tx pgx.Tx, current crmcontracts.Deal,
) (*string, error) {
	if _, err := storekit.LockRow(ctx, tx, dealTable, ids.UUID(current.Id), storekit.LiveOnly); err != nil {
		return nil, err
	}
	var held *string
	if err := tx.QueryRow(ctx,
		`SELECT acquisition_source FROM deal WHERE id = $1`, current.Id).Scan(&held); err != nil {
		return nil, fmt.Errorf("re-read acquisition source: %w", err)
	}
	return held, nil
}
