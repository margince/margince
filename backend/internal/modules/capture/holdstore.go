// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whose mail one seat keeps out of the shared timeline.
//
// A hold is about the CORRESPONDENT, not about any one message: a founder
// holding their lawyer's domain is saying that whatever passes between them is
// nobody else's, without having to read each message to decide. That is the
// difference between this and the confidentiality verdict — the verdict judges
// a conversation, this judges a relationship.
//
// Per USER, never per workspace. One seat's lawyer says nothing about another
// seat's, and a workspace-wide list would let anyone keep a colleague's
// customer out of the shared CRM by naming their domain. The exclusion list
// beside it answers a different question: an exclusion keeps mail OUT of the
// product entirely, a hold captures it and keeps it to the people on it.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The hold vocabulary, the Go spelling of the table's CHECK. The kinds are the
// exclusion kinds by design: both lists answer "which addresses does this rule
// cover", and one spelling is what lets them share ValidExclusionValue rather
// than growing a second parser that folds case differently somewhere.
const (
	HoldKindAddress = ExclusionKindAddress
	HoldKindDomain  = ExclusionKindDomain
)

// CounterpartyHold is one seat's hold as stored.
type CounterpartyHold struct {
	ID        ids.UUID
	UserID    ids.UUID
	Kind      string
	Value     string
	CreatedAt time.Time
}

// CounterpartyHoldStore is the store over one seat's holds.
type CounterpartyHoldStore struct {
	db *database.DB
}

// NewCounterpartyHoldStore binds the store to the pool its writes run through.
func NewCounterpartyHoldStore(db *database.DB) *CounterpartyHoldStore {
	return &CounterpartyHoldStore{db: db}
}

// List answers the acting seat's own holds, and only those.
//
// No id parameter and no scope clause: the statement matches on the
// authenticated user, so there is nothing a caller could pass to read a
// colleague's list. Whose mail a person keeps private is itself private.
func (s *CounterpartyHoldStore) List(ctx context.Context) ([]CounterpartyHold, error) {
	actor, err := seatItself(ctx)
	if err != nil {
		return nil, err
	}
	seat := actor.UserID
	var out []CounterpartyHold
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, user_id, kind, value, created_at
			  FROM capture_counterparty_hold
			 WHERE user_id = $1
			 ORDER BY kind, value`, seat)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h CounterpartyHold
			if err := rows.Scan(&h.ID, &h.UserID, &h.Kind, &h.Value, &h.CreatedAt); err != nil {
				return err
			}
			out = append(out, h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing this seat's counterparty holds: %w", err)
	}
	return out, nil
}

// Add records that this seat's mail with a party is nobody else's.
//
// It holds mail captured from here on. Narrowing what is ALREADY captured is a
// separate, explicit call — a hold placed today is a statement about a
// relationship, and whether it reaches back over a year of shared history is a
// decision the seat makes with the count in front of them.
func (s *CounterpartyHoldStore) Add(ctx context.Context, kind, value string) (CounterpartyHold, error) {
	actor, err := seatItself(ctx)
	if err != nil {
		return CounterpartyHold{}, err
	}
	seat := actor.UserID
	normalized, err := ValidExclusionValue(kind, value)
	if err != nil {
		return CounterpartyHold{}, err
	}
	var h CounterpartyHold
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO capture_counterparty_hold (user_id, kind, value, created_by)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, kind, value) DO UPDATE SET value = EXCLUDED.value
			RETURNING id, user_id, kind, value, created_at`,
			seat, kind, normalized, actor.ID)
		if err := row.Scan(&h.ID, &h.UserID, &h.Kind, &h.Value, &h.CreatedAt); err != nil {
			return err
		}
		// The audit image names the hold and its KIND, never the value: an
		// admin reading the audit log would otherwise learn that this seat
		// corresponds with a divorce lawyer, which is the fact the hold exists
		// to keep private. The id is enough to correlate with the row, and the
		// row is the seat's own to read.
		_, err := storekit.Audit(ctx, tx, "create", captureSettingsObject, h.ID,
			nil, holdAuditImage(h))
		return err
	})
	if err != nil {
		return CounterpartyHold{}, fmt.Errorf("capture: holding a counterparty: %w", err)
	}
	return h, nil
}

// Remove lifts one of the acting seat's holds.
//
// Lifting widens NOTHING that is already captured. Mail held while the hold
// stood was held for a reason that was true at the time, and re-opening a
// year of correspondence as a side effect of tidying a list is not something a
// person asked for. Re-sharing that history is its own call.
func (s *CounterpartyHoldStore) Remove(ctx context.Context, id ids.UUID) error {
	actor, err := seatItself(ctx)
	if err != nil {
		return err
	}
	seat := actor.UserID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var h CounterpartyHold
		// The delete carries the user id, so a caller naming a colleague's hold
		// matches no row and answers as absent — the existence of another
		// seat's hold stays hidden.
		row := tx.QueryRow(ctx, `
			DELETE FROM capture_counterparty_hold
			 WHERE id = $1 AND user_id = $2
			RETURNING id, user_id, kind, value, created_at`, id, seat)
		if err := row.Scan(&h.ID, &h.UserID, &h.Kind, &h.Value, &h.CreatedAt); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "delete", captureSettingsObject, h.ID,
			holdAuditImage(h), nil)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("capture: lifting a counterparty hold: %w", err)
	}
	return nil
}

// holdAuditImage renders a hold for the audit trail: which hold and what kind,
// never the address. See Add for why the value is withheld.
func holdAuditImage(h CounterpartyHold) map[string]any {
	return map[string]any{auditKeyID: h.ID.String(), auditKeyKind: h.Kind}
}
