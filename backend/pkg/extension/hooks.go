// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The hooks a unit runs around core's own writes and operations are part of
// the published extension surface.
//
//margince:extension-surface

package extension

import (
	"context"
	"encoding/json"
)

// Write is one audited write to a core record, as its audit row records it.
// Before and After are the audit images, absent where the write has none.
type Write struct {
	Entity string
	Action string
	ID     string
	Before json.RawMessage
	After  json.RawMessage
}

// WriteHandler runs inside the write's transaction. Returning an error rolls
// the write back. Runtime.Tx joins the same transaction, so the unit's rows
// commit with core's.
type WriteHandler func(ctx context.Context, rt Runtime, w Write) error

// WriteHook asks for the writes of one entity, for the audit actions named.
type WriteHook struct {
	Entity  string
	Actions []string
	Handle  WriteHandler
}

// Operation is one call of a contract operation, before core runs it.
type Operation struct {
	// ID is the operation's operationId in the contract.
	ID string
	// Body is the request's JSON body. A hook may replace it; core then reads
	// the new body as if the caller had sent it.
	Body json.RawMessage
}

// OperationHandler runs before core's operation, outside any transaction.
// Returning an error refuses the call.
type OperationHandler func(ctx context.Context, rt Runtime, op *Operation) error

// OperationHook asks for the calls of the operations named.
type OperationHook struct {
	Operations []string
	Handle     OperationHandler
}
