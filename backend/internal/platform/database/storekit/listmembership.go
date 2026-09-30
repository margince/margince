// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListMemberFilter narrows a record list read to one list's members: it renders
// "this row is a member" over the read's own id column. It carries no row
// scope, because the read it narrows applies its own.
type ListMemberFilter func(idColumn string, arg func(any) int) (string, error)

// ListMemberFilterResolver turns a list id into the narrowing for one record type,
// for a caller who may find the list. It runs BEFORE the read's transaction
// opens: resolving a Live List's filter reads the custom-field catalogue on a
// connection of its own, and a read already holding one must not wait on a
// second from the same pool.
type ListMemberFilterResolver func(ctx context.Context, listID ids.UUID, entityType string) (ListMemberFilter, error)
