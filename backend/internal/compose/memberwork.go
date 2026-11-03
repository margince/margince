// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type memberWork struct{ handoverSeam }

func (s memberWork) Inspect(ctx context.Context, tx pgx.Tx, source ids.UUID, lock bool) (identity.MemberWorkInspection, error) {
	var report identity.MemberWorkInspection
	counts := &report.Owned
	stamp := []string{}
	tally := handoverTally(counts)
	tally[string(recordTypeActivity)] = &counts.Task
	tally["signal"] = &report.Signals
	for _, reference := range slices.Sorted(maps.Keys(memberReferences)) {
		policy := memberReferences[reference]
		if policy.disposition != referenceHandover {
			continue
		}
		table, column, valid := strings.Cut(reference, ".")
		if !valid {
			return report, fmt.Errorf("invalid handover reference %q", reference)
		}
		args := []any{source}
		query := fmt.Sprintf("SELECT id, version FROM %s WHERE %s = $%d AND (%s) ORDER BY id", pgx.Identifier{table}.Sanitize(), pgx.Identifier{column}.Sanitize(), len(args), policy.live)
		if lock {
			query += " FOR UPDATE"
		}
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return report, err
		}
		pins, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
			var id ids.UUID
			var version int64
			if err := row.Scan(&id, &version); err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%d", table, id, version), nil
		})
		if err != nil {
			return report, err
		}
		*tally[table] = len(pins)
		stamp = append(stamp, pins...)
	}
	visible, err := s.countOwned(ctx, tx, source)
	if err != nil {
		return report, err
	}
	report.VisibleOwned = visible
	blockers, err := memberRetirementBlockers(ctx, tx, source)
	report.Blockers = blockers
	slices.Sort(stamp)
	report.Stamp = strings.Join(stamp, ";")
	return report, err
}

func (s memberWork) Retire(ctx context.Context, tx pgx.Tx, source ids.UUID, target *ids.UUID, merge bool) error {
	if merge && target != nil {
		ctx = auth.WithConsolidatedMembers(ctx, source, *target)
	}
	before, err := s.Inspect(ctx, tx, source, true)
	if err != nil {
		return err
	}
	work := before.Owned
	if err := s.transferMemberWork(ctx, tx, source, target, merge, work); err != nil {
		return err
	}

	if merge {
		if target == nil {
			return apperrors.ErrConflict
		}
		if err := signals.TransferMemberSignalsTx(ctx, tx, source, *target); err != nil {
			return err
		}
	} else if err := signals.ArchiveMemberSignalsTx(ctx, tx, source); err != nil {
		return err
	}
	after, err := s.Inspect(ctx, tx, source, true)
	left := after.Owned
	if err != nil {
		return err
	}
	if left.Contact+left.Company+left.Lead+left.Deal+left.Project+left.Task+after.Signals > 0 {
		return fmt.Errorf("%w: some work could not be transferred; nothing was removed", apperrors.ErrConflict)
	}
	return nil
}

func (s memberWork) transferMemberWork(ctx context.Context, tx pgx.Tx, source ids.UUID, target *ids.UUID, merge bool, work crmcontracts.HandoverCounts) error {
	if work.Contact+work.Company+work.Lead+work.Deal+work.Project+work.Task == 0 {
		return nil
	}
	if target == nil {
		return apperrors.ErrConflict
	}
	if merge {
		if _, err := s.tasks.ConsolidateMemberTasksTx(ctx, tx, ids.From[ids.UserKind](source), ids.From[ids.UserKind](*target)); err != nil {
			return err
		}
	}
	_, err := s.handOverInTx(ctx, tx, source, ids.From[ids.UserKind](*target))
	return err
}
