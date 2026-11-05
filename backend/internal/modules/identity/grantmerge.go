// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A merge joins two records of one real contact, company or lead. A share
// given on the retired record follows it onto the survivor. Contacts owns the
// merge and asks for this inside its own transaction. Identity owns the grant
// rows and the audit shape every grant change is recorded in.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CarryGrantsTx moves every grant on the retired record onto the survivor.
// Where the grantee already holds a grant on the survivor, the two become one.
// Each change is audited on the record it changed, with the other record named
// in the evidence.
func (s *Service) CarryGrantsTx(ctx context.Context, tx pgx.Tx, recordType string, from, to ids.UUID) error {
	// The merge verb is an update of the record type, and so is a share.
	if err := admitGrantCarry(ctx, tx, recordType, from, to); err != nil {
		return err
	}
	retired, err := grantsOnRecord(ctx, tx, recordType, from)
	if err != nil {
		return err
	}
	if len(retired) == 0 {
		return nil
	}
	// The visibility predicates judge expiry against the database clock, so
	// the fold does too.
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return fmt.Errorf("read the database clock: %w", err)
	}
	for _, grant := range retired {
		if err := carryGrant(ctx, tx, grant, to, now); err != nil {
			return err
		}
	}
	return nil
}

// grantsOnRecord locks and reads the grants on one record, in id order so two
// merges touching the same rows lock them in the same order.
func grantsOnRecord(ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID) ([]grantRow, error) {
	rows, err := tx.Query(ctx, "SELECT "+grantColumns+` FROM record_grant
		WHERE `+grantRecordScope+` ORDER BY id FOR UPDATE`, pgx.NamedArgs{grantRecordTypeArg: recordType, grantRecordIDArg: recordID})
	if err != nil {
		return nil, fmt.Errorf("read the grants on the retired %s: %w", recordType, err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (grantRow, error) { return scanGrant(r) })
}

// carryGrant moves one grant, or folds it into the grant its grantee already
// holds on the survivor. It takes the write-identity lock CreateRecordGrant
// takes. A share asserted on the survivor at the same moment then waits instead
// of adding a second row under the unique key.
func carryGrant(ctx context.Context, tx pgx.Tx, retired grantRow, to ids.UUID, now time.Time) error {
	onSurvivor := grantTarget(retired, to)
	if err := storekit.LockWriteIdentity(ctx, tx, "record_grant", grantIdentity(onSurvivor)); err != nil {
		return err
	}
	kept, held, err := replacedGrant(ctx, tx, onSurvivor)
	if err != nil {
		return err
	}
	if !held {
		return moveGrant(ctx, tx, retired, to)
	}
	return foldGrant(ctx, tx, kept, retired, now)
}

// moveGrant re-points one grant at the survivor. It records an unshare on the
// retired record and a share on the survivor. Each record's history then says
// who could reach it and when that changed.
func moveGrant(ctx context.Context, tx pgx.Tx, retired grantRow, to ids.UUID) error {
	moved, err := scanGrant(tx.QueryRow(ctx, `UPDATE record_grant SET record_id = $2, version = version + 1
		WHERE id = $1 AND version = $3 RETURNING `+grantColumns, retired.ID, to, retired.Version))
	if err != nil {
		return fmt.Errorf("move the grant onto the survivor: %w", err)
	}
	if err := auditUnshareByMerge(ctx, tx, retired, to); err != nil {
		return err
	}
	if _, err := storekit.AuditWithEvidence(ctx, tx, "record_share", moved.RecordType, moved.RecordID,
		nil, grantImage(moved), map[string]any{"merged_from": retired.RecordID}); err != nil {
		return fmt.Errorf("audit the grant the merge moved: %w", err)
	}
	return nil
}

// foldGrant makes one grant of the two a grantee holds, then deletes the
// retired one. The survivor's row is rewritten only when the fold changed it.
func foldGrant(ctx context.Context, tx pgx.Tx, kept, retired grantRow, now time.Time) error {
	folded := strongerGrant(kept, retired, now)
	if folded != kept {
		updated, err := scanGrant(tx.QueryRow(ctx, `UPDATE record_grant
			SET access = $2, expires_at = $3, reason = $4, granted_by = $5, version = version + 1
			WHERE id = $1 AND version = $6 RETURNING `+grantColumns,
			kept.ID, folded.Access, folded.ExpiresAt, folded.Reason, folded.GrantedBy, kept.Version))
		if err != nil {
			return fmt.Errorf("fold the retired grant into the survivor's: %w", err)
		}
		if _, err := storekit.AuditWithEvidence(ctx, tx, "record_share", kept.RecordType, kept.RecordID,
			grantImage(kept), grantImage(updated), map[string]any{"merged_from": retired.RecordID}); err != nil {
			return fmt.Errorf("audit the folded grant: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM record_grant WHERE id = $1`, retired.ID); err != nil {
		return fmt.Errorf("drop the folded grant: %w", err)
	}
	return auditUnshareByMerge(ctx, tx, retired, kept.RecordID)
}

func auditUnshareByMerge(ctx context.Context, tx pgx.Tx, retired grantRow, to ids.UUID) error {
	if _, err := storekit.AuditWithEvidence(ctx, tx, "record_unshare", retired.RecordType, retired.RecordID,
		grantImage(retired), nil, map[string]any{"merged_into": to}); err != nil {
		return fmt.Errorf("audit the grant the merge took off the retired record: %w", err)
	}
	return nil
}

// strongerGrant answers the one grant a grantee keeps on the survivor. An
// expired grant confers nothing, so it never lifts the other. Between two live
// grants the result has the higher access and the later expiry, where no
// expiry outlasts any date. The survivor's reason and grantor stay unless the
// retired grant is the only live one, and then it is taken whole.
func strongerGrant(kept, retired grantRow, now time.Time) grantRow {
	switch {
	case !grantLive(retired, now):
		return kept
	case !grantLive(kept, now):
		kept.Access, kept.ExpiresAt = retired.Access, retired.ExpiresAt
		kept.Reason, kept.GrantedBy = retired.Reason, retired.GrantedBy
		return kept
	}
	if retired.Access == string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite) {
		kept.Access = retired.Access
	}
	kept.ExpiresAt = laterExpiry(kept.ExpiresAt, retired.ExpiresAt)
	return kept
}

func grantLive(g grantRow, now time.Time) bool {
	return g.ExpiresAt == nil || g.ExpiresAt.After(now)
}

func laterExpiry(a, b *time.Time) *time.Time {
	if a == nil || b == nil {
		return nil
	}
	if b.After(*a) {
		return b
	}
	return a
}
