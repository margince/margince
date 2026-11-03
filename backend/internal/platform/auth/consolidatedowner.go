// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type (
	consolidatedOwnerKey struct{}
	consolidatedMembers  struct{ source, target ids.UUID }
)

// WithConsolidatedMembers permits the selected account to retain duplicate work
// before sign-in setup; the caller still needs every ordinary assignment grant.
func WithConsolidatedMembers(ctx context.Context, source, target ids.UUID) context.Context {
	return context.WithValue(ctx, consolidatedOwnerKey{}, consolidatedMembers{source, target})
}

func consolidatingInto(ctx context.Context, target ids.UUID) bool {
	selected, ok := ctx.Value(consolidatedOwnerKey{}).(consolidatedMembers)
	return ok && selected.target == target
}

// ConsolidatingMember binds the metadata-only transfer to the approved identity pair.
func ConsolidatingMember(ctx context.Context, source, target ids.UUID) bool {
	selected, ok := ctx.Value(consolidatedOwnerKey{}).(consolidatedMembers)
	return ok && selected.source == source && selected.target == target
}
