// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// WithListMembers lets the deal list narrow to one list's members. Lists
// belong to another module, so compose injects the resolver, and only while
// the installation has lists switched on.
func (h Handlers) WithListMembers(resolve storekit.ListMemberFilterResolver) Handlers {
	h.listMembers = resolve
	return h
}

// narrowToList resolves the list a deal read names into its member narrowing.
// The HTTP door, the seam and the filter-reach probe share it, so they refuse list_id alike.
func narrowToList(ctx context.Context, in *ListDealsInput, resolve storekit.ListMemberFilterResolver) error {
	membership, err := storekit.ResolveListMembership(ctx, in.MemberOf, resolve, dealListEntity)
	if err != nil {
		return err
	}
	in.Membership = membership
	return nil
}
