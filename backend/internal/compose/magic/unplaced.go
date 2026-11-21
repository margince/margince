// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UnplacedChange counts one kind of admitted machine change the receipt cannot
// place. Action is the audit action, or the link a list or tag row filed.
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
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		changes, err = unplacedChangesSince(ctx, tx, since)
		return err
	})
	return changes, err
}

// unplacedChangesSince counts the admitted machine changes no done-lane arm can
// place. With no table to join, no gate proves the reader may see them. A
// child row a placement can place is placed, whoever reads.
// Creates and archives are left out, since they include bookkeeping. Upkeep of
// maintenance subjects is left out, except the record links filed under them.
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
	action, linked := linkChangeSQL(arg)
	query := fmt.Sprintf(`
		SELECT a.entity_type, %s, count(*) FROM audit_log a
		 WHERE a.occurred_at >= $%d
		   AND a.actor_type = ANY($%d)
		   AND a.action = ANY($%d)
		   AND (NOT (a.entity_type = ANY($%d)) OR %s)
		   AND NOT (a.entity_type = ANY($%d))
		   AND NOT %s
		 GROUP BY 1, 2
		 ORDER BY 3 DESC, 1, 2`,
		action, arg(since), arg(MachineActors()), arg(admittedActions(false)),
		arg(auditentity.MaintenanceTypes()), linked, arg(placed), placeableSQL(arg))
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

// linkChangeSQL answers the action a row counts under and the predicate that
// admits it. A list or tag row counts under the link it filed.
func linkChangeSQL(arg func(any) int) (string, string) {
	var action strings.Builder
	action.WriteString("CASE")
	linked := make([]string, 0, len(auditentity.LinkChanges()))
	for _, link := range auditentity.LinkChanges() {
		key := arg(link.Key)
		match := fmt.Sprintf("(a.entity_type = $%d AND a.after ? $%d)", arg(link.EntityType), key)
		fmt.Fprintf(&action, " WHEN %s THEN $%d::text", match, key)
		linked = append(linked, match)
	}
	action.WriteString(" ELSE a.action END")
	return action.String(), "(" + strings.Join(linked, " OR ") + ")"
}
