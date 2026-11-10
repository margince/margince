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
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

type memberWork struct{ handoverSeam }

func (s memberWork) Inspect(ctx context.Context, tx pgx.Tx, source ids.UUID, target *ids.UUID, lock bool) (identity.MemberWorkInspection, error) {
	var report identity.MemberWorkInspection
	counts := &report.Owned
	stamp := []string{}
	member, present, err := memberWorkReceiverTx(ctx, tx, target)
	if err != nil {
		return report, err
	}
	var receiver *principal.Principal
	if present {
		receiver = &member
	}

	tally := handoverTally(counts)
	tally[string(recordTypeActivity)] = &counts.Task
	tally["signal"] = &report.Signals
	seen := map[string]bool{}
	for _, reference := range slices.Sorted(maps.Keys(memberReferences)) {
		policy := memberReferences[reference]
		if policy.disposition != referenceHandover {
			continue
		}
		table, _, valid := strings.Cut(reference, ".")
		if !valid {
			return report, fmt.Errorf("invalid handover reference %q", reference)
		}
		if seen[table] {
			continue
		}
		seen[table] = true
		if table == handoverAssignmentTable || table == handoverHandoffTable {
			snapshot, err := inspectMemberStaffingTx(ctx, tx, table, source, receiver)
			if err != nil {
				return report, err
			}
			*tally[table] = snapshot.Owned
			*handoverTally(&report.Transferable)[table] = snapshot.Transferable
			stamp = append(stamp, snapshot.Stamp...)
			continue
		}
		pins, err := memberReferencePinsTx(ctx, tx, table, source, lock)
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

func memberReferencePinsTx(ctx context.Context, tx pgx.Tx, table string, source ids.UUID, lock bool) ([]string, error) {
	query := fmt.Sprintf("SELECT id, version FROM %s WHERE %s ORDER BY id", pgx.Identifier{table}.Sanitize(), memberHandoverPredicate(table))
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := tx.Query(ctx, query, pgx.StrictNamedArgs{memberSourceArgument: source})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
		var id ids.UUID
		var version int64
		if err := row.Scan(&id, &version); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s:%s:%d", table, id, version), nil
	})
}

func memberWorkReceiverTx(ctx context.Context, tx pgx.Tx, target *ids.UUID) (principal.Principal, bool, error) {
	if target == nil {
		return principal.Principal{}, false, nil
	}
	members, err := identity.MemberPrincipalsTx(ctx, tx, []ids.UUID{*target})
	if err != nil {
		return principal.Principal{}, false, err
	}
	receiver, exists := members[*target]
	return receiver, exists, nil
}

func inspectMemberStaffingTx(ctx context.Context, tx pgx.Tx, table string, source ids.UUID, receiver *principal.Principal) (workforce.MemberWorkSnapshot, error) {
	switch table {
	case handoverAssignmentTable:
		return assignments.InspectMemberAssignmentsTx(ctx, tx, source, receiver)
	case handoverHandoffTable:
		return contacts.InspectMemberHandoffsTx(ctx, tx, source, receiver)
	default:
		return workforce.MemberWorkSnapshot{}, apperrors.ErrInvalidArgument
	}
}

func memberHandoverPredicate(table string) string {
	var clauses []string
	for _, reference := range slices.Sorted(maps.Keys(memberReferences)) {
		policy := memberReferences[reference]
		name, column, valid := strings.Cut(reference, ".")
		if valid && name == table && policy.disposition == referenceHandover {
			clauses = append(clauses, "("+pgx.Identifier{column}.Sanitize()+" = @source_id AND ("+policy.live+"))")
		}
	}
	return strings.Join(clauses, " OR ")
}

func (s memberWork) Retire(ctx context.Context, tx pgx.Tx, source ids.UUID, target *ids.UUID, merge bool) error {
	if merge && target != nil {
		ctx = auth.WithConsolidatedMembers(ctx, source, *target)
	}
	if err := assignments.ArchiveRetiredMemberAssignmentsTx(ctx, tx, source); err != nil {
		return err
	}
	before, err := s.Inspect(ctx, tx, source, target, true)
	if err != nil {
		return err
	}
	work := before.Owned
	if err := s.transferMemberWork(ctx, tx, source, target, merge, work); err != nil {
		return err
	}
	if err := collections.ArchiveMemberSavedViewsTx(ctx, tx, ids.From[ids.UserKind](source)); err != nil {
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
	after, err := s.Inspect(ctx, tx, source, target, true)
	left := after.Owned
	if err != nil {
		return err
	}
	if handoverWorkCount(left)+after.Signals > 0 {
		return fmt.Errorf("%w: some work could not be transferred; nothing was removed", apperrors.ErrConflict)
	}
	return nil
}

func (s memberWork) transferMemberWork(ctx context.Context, tx pgx.Tx, source ids.UUID, target *ids.UUID, merge bool, work crmcontracts.HandoverCounts) error {
	if handoverWorkCount(work) == 0 {
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
