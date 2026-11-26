// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const afterCursorArg = "after"

const unresolvedEmploymentScope = `c.archived_at IS NULL AND EXISTS(SELECT 1 FROM provider_employment_resolution r WHERE r.contact_id=c.id AND r.state='needs_review')`

// CountUnresolvedEmploymentContacts caps estimates before the repair is approved.
func CountUnresolvedEmploymentContacts(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT c.id FROM contact c WHERE `+unresolvedEmploymentScope+` LIMIT @limit) candidates`, pgx.NamedArgs{signaturePhoneLimitParam: maximum + 1}).Scan(&n)
	return min(n, maximum), n > maximum, err
}

// RepairUndatedEmploymentTx bounds work by contacts and reuses the import decision for each episode.
func (s *Store) RepairUndatedEmploymentTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id FROM contact c WHERE c.id>@after AND `+unresolvedEmploymentScope+` ORDER BY c.id LIMIT @limit`, pgx.NamedArgs{afterCursorArg: after, signaturePhoneLimitParam: limit})
	if err != nil {
		return nil, 0, err
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, err
	}
	changed := 0
	for _, id := range scanned {
		moved, err := s.repairUndatedEmploymentContactTx(ctx, tx, id)
		if err != nil {
			return nil, 0, err
		}

		if moved {
			changed++
		}
	}
	return scanned, changed, nil
}

func repairableEmploymentEpisode(e employmentEvidence) bool {
	return e.Status == employmentCurrent && e.Started != "" && !e.invalidDates && !e.superseded
}

func (s *Store) repairUndatedEmploymentContactTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	contact := ids.From[ids.ContactKind](id)
	if err := auth.HoldWritableLive(ctx, tx, contactEntity, id); err != nil {
		return false, err
	}
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contact.String()); err != nil {
		return false, err
	}
	episodes, err := readEmploymentEvidence(ctx, tx, contact)
	if err != nil {
		return false, err
	}
	moved := false
	for _, e := range episodes {
		if !repairableEmploymentEpisode(e) {
			continue
		}
		prior, err := employmentImportItem(ctx, tx, contact, e)
		if err != nil {
			return false, err
		}
		if prior.State != crmcontracts.EmploymentImportItemStateNeedsReview {
			continue
		}
		if err := s.applyEmploymentEpisode(ctx, tx, contact, e, crmcontracts.EmploymentImportRequest{}, employmentApply); err != nil {
			return false, err
		}
		current, err := employmentImportItem(ctx, tx, contact, e)
		if err != nil {
			return false, err
		}
		if current.State == employmentLinked {
			moved = true
		}
	}
	return moved, nil
}
