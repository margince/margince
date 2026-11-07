// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func riskViewOwners(ctx context.Context, owner *ids.UUID) *storekit.OwnerSet {
	owners := scheduledRunOwners(ctx)
	if owner == nil {
		return owners
	}
	if owners != nil && !slices.Contains(owners.IDs, *owner) {
		return &storekit.OwnerSet{}
	}
	return &storekit.OwnerSet{IDs: []ids.UUID{*owner}}
}
