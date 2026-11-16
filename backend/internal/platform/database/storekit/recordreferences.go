// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReadRecordReferences collects an already-authorized identity query inside the caller's bound transaction.
// An empty statement follows the caller's object-grant check without opening a transaction.
func ReadRecordReferences(ctx context.Context, store TxRunner, statement string, args ...any) ([]crmcontracts.RecordReference, error) {
	references := []crmcontracts.RecordReference{}
	if statement == "" {
		return references, nil
	}
	err := store.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, statement, args...)
		if err != nil {
			return err
		}
		references, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.RecordReference, error) {
			var reference crmcontracts.RecordReference
			var id ids.UUID
			err := row.Scan(&id, &reference.Name, &reference.Archived)
			reference.Id = openapi_types.UUID(id)
			return reference, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("reading record references: %w", err)
	}
	return references, nil
}
