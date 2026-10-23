// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// handoverObject is the grant a member handover is admitted on.
const handoverObject = "user_admin"

// PrivateCounts are a member's live owner-private contacts and companies.
type PrivateCounts struct {
	Contacts  int
	Companies int
}

// privateHandoverTables are the tables whose rows a capture keeps owner-private.
var privateHandoverTables = []string{contactEntity, companyEntity}

// OwnerPrivateCountsTx counts the live owner-private contacts and companies a
// member who left still holds; zero for a member who has not left. Behind the
// member handover grant: the numbers are all an administrator learns about rows
// only their owner can read.
func OwnerPrivateCountsTx(ctx context.Context, tx pgx.Tx, owner ids.UserID) (PrivateCounts, error) {
	if err := auth.Require(ctx, handoverObject, principal.ActionUpdate); err != nil {
		return PrivateCounts{}, err
	}
	if left, err := departed(ctx, tx, owner); err != nil || !left {
		return PrivateCounts{}, err
	}
	var counts PrivateCounts
	for _, table := range privateHandoverTables {
		ownedIDs, err := ownerPrivateIDs(ctx, tx, table, owner, false)
		if err != nil {
			return PrivateCounts{}, err
		}
		counts.set(table, len(ownedIDs))
	}
	return counts, nil
}

// HandOverOwnerPrivateTx moves a departed member's live owner-private contacts
// and companies to to. They stay owner-private, now to the new owner. A member
// who has not left keeps theirs.
//
// Not through updateContactInTx: that edit is admitted by row visibility, and a
// departed owner's private rows are visible to nobody. This write changes the
// owner column alone, plus its audit row and its updated event.
func HandOverOwnerPrivateTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (PrivateCounts, error) {
	if err := auth.Require(ctx, handoverObject, principal.ActionUpdate); err != nil {
		return PrivateCounts{}, err
	}
	if err := auth.EnsureAssignee(ctx, tx, to.UUID); err != nil {
		return PrivateCounts{}, err
	}
	if left, err := departed(ctx, tx, from); err != nil || !left {
		return PrivateCounts{}, err
	}
	var moved PrivateCounts
	for _, table := range privateHandoverTables {
		ownedIDs, err := ownerPrivateIDs(ctx, tx, table, from, true)
		if err != nil {
			return PrivateCounts{}, err
		}
		for _, id := range ownedIDs {
			if err := handOverPrivateRow(ctx, tx, table, id, from, to); err != nil {
				return PrivateCounts{}, err
			}
		}
		moved.set(table, len(ownedIDs))
	}
	return moved, nil
}

// departed reports whether member left. The handover holds the member's row
// lock, so the answer stands for the rest of its transaction.
func departed(ctx context.Context, tx pgx.Tx, member ids.UserID) (bool, error) {
	var left bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1 AND `+workforce.DepartedMemberSQL("")+`)`,
		member).Scan(&left)
	if err != nil {
		return false, fmt.Errorf("ask whether %s left: %w", member, err)
	}
	return left, nil
}

func (c *PrivateCounts) set(table string, n int) {
	if table == contactEntity {
		c.Contacts = n
		return
	}
	c.Companies = n
}

// ownerPrivateIDs lists owner's live owner-private rows of table in id order,
// locked when lock is set so two handovers queue rather than deadlock.
func ownerPrivateIDs(ctx context.Context, tx pgx.Tx, table string, owner ids.UserID, lock bool) ([]ids.UUID, error) {
	query := `SELECT id FROM ` + pgx.Identifier{table}.Sanitize() +
		` WHERE owner_id = $1 AND visibility = 'owner' AND archived_at IS NULL ORDER BY id`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, query, owner)
	if err != nil {
		return nil, fmt.Errorf("list owner-private %s rows: %w", table, err)
	}
	ownedIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("list owner-private %s rows: %w", table, err)
	}
	return ownedIDs, nil
}

// handOverPrivateRow names its table as a constant in each arm, so the
// table-ownership gate can attribute the write to this module.
func handOverPrivateRow(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, from, to ids.UserID) error {
	var lock storekit.RowLock
	var err error
	if table == contactEntity {
		lock, err = storekit.LockRow(ctx, tx, contactEntity, id, storekit.LiveOnly)
	} else {
		lock, err = storekit.LockRow(ctx, tx, companyEntity, id, storekit.LiveOnly)
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock %s %s for a handover: %w", table, id, err)
	}
	p := storekit.NewPatch()
	p.Set(ownerIDColumn, from, to)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("hand over %s %s: %w", table, id, err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", table, id, p.Before(), p.After())
	if err != nil {
		return fmt.Errorf("audit %s handover: %w", table, err)
	}
	if table == contactEntity {
		return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventContactUpdated{ChangedFields: p.After()})
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventCompanyUpdated{ChangedFields: p.After()})
}
