// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// ExplainCachedStatement measures the statement on the backend that prepared it.
func ExplainCachedStatement(ctx context.Context, tx pgx.Tx, query string, values []any) ([]byte, error) {
	if len(values) == 1 {
		if named, ok := values[0].(pgx.NamedArgs); ok {
			var err error
			query, values, err = named.RewriteQuery(ctx, tx.Conn(), query, nil)
			if err != nil {
				return nil, err
			}
		}
	}
	var name string
	var types []uint32
	if err := tx.QueryRow(ctx, `SELECT name,parameter_types::oid[] FROM pg_prepared_statements WHERE statement=@query`, pgx.NamedArgs{"query": query}).Scan(&name, &types); err != nil {
		return nil, fmt.Errorf("locating the measured cached statement: %w", err)
	}
	if len(types) != len(values) {
		return nil, fmt.Errorf("cached statement has %d parameters for %d values", len(types), len(values))
	}
	encoded := make([]any, len(values))
	for i, value := range values {
		raw, err := tx.Conn().TypeMap().Encode(types[i], pgx.TextFormatCode, value, nil)
		if err != nil {
			return nil, fmt.Errorf("encoding cached parameter %d: %w", i, err)
		}
		if raw != nil {
			encoded[i] = string(raw)
		}
	}
	var raw []byte
	explain := `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) EXECUTE ` + pgx.Identifier{name}.Sanitize() + `(` + database.Placeholders(values) + `)`
	err := tx.QueryRow(ctx, explain, append([]any{pgx.QueryExecModeSimpleProtocol}, encoded...)...).Scan(&raw)
	return raw, err
}
