// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// operationalPruneBatch is how many rows one transaction of a prune removes.
const operationalPruneBatch = 5000

// OperationalPrune removes one module's operational rows past their window.
// These rows are bookkeeping, not business records, so no hold or floor keeps
// them. Remove deletes up to limit rows from before cutoff and answers how many.
type OperationalPrune struct {
	Name   string
	Days   int
	Remove func(ctx context.Context, tx pgx.Tx, cutoff time.Time, limit int) (int64, error)
}

// WithOperationalPrunes returns a service whose nightly pass runs these
// prunes. The owning module supplies each one, because it owns the table.
func (s *RetentionService) WithOperationalPrunes(prunes ...OperationalPrune) *RetentionService {
	out := *s
	out.operationalPrunes = prunes
	return &out
}

// pruneOperationalRows runs each prune a page per transaction until a page
// comes back short.
func (s *RetentionService) pruneOperationalRows(ctx context.Context) error {
	for _, prune := range s.operationalPrunes {
		cutoff := time.Now().AddDate(0, 0, -prune.Days)
		var total int64
		for {
			var removed int64
			err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
				removed, err = prune.Remove(ctx, tx, cutoff, operationalPruneBatch)
				return err
			})
			if err != nil {
				return fmt.Errorf("retention %s: %w", prune.Name, err)
			}
			total += removed
			if removed < operationalPruneBatch {
				break
			}
		}
		s.log.Info("retention: operational rows past their window removed", "rows", prune.Name, "count", total)
	}
	return nil
}
