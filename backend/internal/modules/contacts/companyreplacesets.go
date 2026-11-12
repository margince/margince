// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyReplaceSetsChanged answers whether the staged domain and
// relationship-type sets differ from the live rows.
//
// The company form sends both sets on every save, so a save of an untouched
// company carries them. The staged sets are already normalised and have their
// primary elected, so they compare directly with the live rows.
func companyReplaceSetsChanged(ctx context.Context, tx pgx.Tx, id ids.CompanyID, in UpdateCompanyInput) (bool, error) {
	if in.RelationshipTypes != nil || in.ExtensionRelationshipTypes != nil {
		all, err := readLiveRelationshipTypes(ctx, tx, id)
		if err != nil {
			return false, err
		}
		live, liveExt := splitRelationshipTypes(all)
		if in.RelationshipTypes != nil && !sameStringSet(live, *in.RelationshipTypes) {
			return true, nil
		}
		if in.ExtensionRelationshipTypes != nil && !sameStringSet(liveExt, *in.ExtensionRelationshipTypes) {
			return true, nil
		}
	}
	if in.Domains == nil {
		return false, nil
	}
	live, _, primary, err := readLiveDomains(ctx, tx, id)
	if err != nil {
		return false, err
	}
	desired := *in.Domains
	if len(desired) != len(live) {
		return true, nil
	}
	for _, d := range desired {
		if !live[d.Domain] || (d.IsPrimary && d.Domain != primary) {
			return true, nil
		}
	}
	return false, nil
}

func sameStringSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
