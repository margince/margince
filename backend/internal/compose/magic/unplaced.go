// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UnplacedChange counts one kind of admitted machine change the receipt cannot
// place, by its audit action.
type UnplacedChange struct {
	EntityType string
	Action     string
	Count      int
}

// UnplacedTotal is the receipt's unknown_entity_type count.
func UnplacedTotal(changes []UnplacedChange) int {
	total := 0
	for _, change := range changes {
		total += change.Count
	}
	return total
}

// maxUnplacedKinds is the contract's bound on System health's breakdown.
const maxUnplacedKinds = 50

// UnplacedBreakdown keeps the first maxUnplacedKinds kinds and the whole total.
func UnplacedBreakdown(changes []UnplacedChange) (int, []crmcontracts.HealthUnplacedChange) {
	kept := changes[:min(len(changes), maxUnplacedKinds)]
	breakdown := make([]crmcontracts.HealthUnplacedChange, 0, len(kept))
	for _, change := range kept {
		breakdown = append(breakdown, crmcontracts.HealthUnplacedChange{EntityType: change.EntityType, Action: change.Action, Count: change.Count})
	}
	return UnplacedTotal(changes), breakdown
}

// UnplacedChanges is the breakdown System health reads, outside any receipt.
func (s *Service) UnplacedChanges(ctx context.Context, since time.Time) ([]UnplacedChange, error) {
	var changes []UnplacedChange
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		changes, err = unplacedChangesSince(ctx, tx, since)
		return err
	})
	return changes, err
}

// unplacedChangesSince counts the admitted machine changes no done-lane arm can
// place. With no table to join, no gate proves the reader may see them. A
// child row a placement can place is placed, whoever reads.
// Creates and archives are left out, since they include bookkeeping. Changes to
// maintenance and upkeep subjects are left out, list and tag links included.
// The counts name what kinds of record were touched, so they need ai_diagnostics.
func unplacedChangesSince(ctx context.Context, tx pgx.Tx, since time.Time) ([]UnplacedChange, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return nil, err
	}
	placed := make([]string, 0, len(scopedTypes)+1)
	for entityType := range scopedTypes {
		placed = append(placed, entityType)
	}
	placed = append(placed, typeActivity)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := fmt.Sprintf(`
		SELECT a.entity_type, a.action, count(*) FROM audit_log a
		 WHERE a.occurred_at >= $%d
		   AND a.actor_type = ANY($%d)
		   AND a.action = ANY($%d)
		   AND NOT (a.entity_type = ANY($%d))
		   AND NOT (a.entity_type = ANY($%d))
		   AND NOT %s
		 GROUP BY 1, 2
		 ORDER BY 3 DESC, 1, 2`,
		arg(since), arg(MachineActors()), arg(admittedActions(false)),
		arg(slices.Concat(auditentity.MaintenanceTypes(), auditentity.UpkeepTypes())), arg(placed), placeableSQL(arg))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count the machine actions this build cannot place: %w", err)
	}
	defer rows.Close()
	changes := []UnplacedChange{}
	for rows.Next() {
		var change UnplacedChange
		if err := rows.Scan(&change.EntityType, &change.Action, &change.Count); err != nil {
			return nil, fmt.Errorf("read a machine action this build cannot place: %w", err)
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}
