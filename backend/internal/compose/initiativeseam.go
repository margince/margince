// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// initiativeRules evaluates an initiative's rule through the list filter
// engine of the record type. So an initiative and a dynamic list answer one
// predicate the same way, under the same row scope.
type initiativeRules struct{ collections *collections.Store }

func (r initiativeRules) Match(ctx context.Context, tx pgx.Tx, recordType string, rule storekit.Predicate, limit int) ([]ids.UUID, int, error) {
	engine, ok, err := r.collections.SegmentEngine(ctx, recordType)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return nil, 0, fmt.Errorf("%q is not a filterable record type: %w", recordType, apperrors.ErrInvalidArgument)
	}
	count, err := engine.CountMatching(ctx, tx, rule)
	if err != nil {
		return nil, 0, err
	}
	matched, err := engine.SelectIDs(ctx, tx, rule, limit)
	if err != nil {
		return nil, 0, err
	}
	return matched, count, nil
}

// newInitiativeStore is the one initiative store the transport and the
// reporting engine share, so both read one membership.
func newInitiativeStore(pool *pgxpool.Pool) *initiatives.Store {
	db := InstallationDB(pool)
	return initiatives.NewStore(db, time.Now, initiativeRules{collections: collections.NewStore(db)})
}
