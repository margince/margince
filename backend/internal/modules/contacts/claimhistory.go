// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CustomerPromiseIdentity links source evidence to its previously attributed customer.
type CustomerPromiseIdentity struct {
	Locator string
	Contact ids.UUID
}

// CustomerPromiseHistoryTx preserves visible attribution history when a source is read again.
func (s *Store) CustomerPromiseHistoryTx(ctx context.Context, tx pgx.Tx, source ids.UUID) ([]CustomerPromiseIdentity, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	content, err := claimContentClause(ctx, arg)
	if err != nil {
		return nil, err
	}
	contact, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return nil, err
	}
	if contact == "" {
		contact = sqlAlwaysVisible
	}
	query := fmt.Sprintf(`SELECT c.evidence_fingerprint, c.contact_id
 FROM conversation_claim c JOIN activity a ON a.id=c.source_activity_id AND a.archived_at IS NULL
 JOIN contact p ON p.id=c.contact_id
 WHERE c.source_activity_id=$%d AND c.kind='commitment_theirs' AND (%s) AND (%s)`, arg(source), content, contact)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read customer promise identities: %w", err)
	}
	defer rows.Close()
	var history []CustomerPromiseIdentity
	for rows.Next() {
		var item CustomerPromiseIdentity
		if err := rows.Scan(&item.Locator, &item.Contact); err != nil {
			return nil, fmt.Errorf("decode customer promise identity: %w", err)
		}
		history = append(history, item)
	}
	return history, rows.Err()
}
