// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithListMembers lets the contact, company and lead lists narrow to one
// list's members. Lists belong to another module, so compose injects the
// resolver, and only while the installation has lists switched on.
func (h Handlers) WithListMembers(resolve storekit.ListMemberFilterResolver) Handlers {
	h.listMembers = resolve
	return h
}

// memberFilter resolves a list_id parameter the caller sent for one record type. With lists
// switched off a list id names nothing, so it answers not found.
func (h Handlers) memberFilter(ctx context.Context, listID openapi_types.UUID, entityType string) (storekit.ListMemberFilter, error) {
	if h.listMembers == nil {
		return nil, apperrors.ErrNotFound
	}
	return h.listMembers(ctx, ids.UUID(listID), entityType)
}
