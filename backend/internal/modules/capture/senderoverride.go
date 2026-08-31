// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a person decided about a sender, and the rule that the machine never
// takes it back.
//
// The verdict engine judges every new sender and is sometimes wrong — it calls
// a customer's private address personal, or an old friend business. An owner
// correcting that has to be corrected permanently: a decision the next message
// overwrites is not a decision, it is a suggestion, and the owner would find
// the same sender wrong again next week with no way to tell why.
//
// Per SEAT. A sender is personal to the person who knows them, so one rep's
// family member is another rep's customer, and a shared list would let either
// overrule the other about their own correspondence.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The decisions a person may record about a sender.
const (
	// OverrideBusiness readmits a sender the machine judged noise: they are a
	// counterparty of this business after all, and their mail belongs in the
	// CRM like anyone else's.
	OverrideBusiness = "business"
	// OverrideKeepOut ends it: no record, and the mail this sender already
	// brought in is destroyed.
	OverrideKeepOut = "keep_out"
)

// SenderOverride is one seat's decision about one address.
type SenderOverride struct {
	ID            ids.UUID
	Address       string
	Decision      string
	OverruledKind string
}

// SenderOverrideStore holds what people decided.
type SenderOverrideStore struct {
	db *database.DB
}

// NewSenderOverrideStore builds the store.
func NewSenderOverrideStore(db *database.DB) *SenderOverrideStore {
	return &SenderOverrideStore{db: db}
}

// Set records the caller's decision about a sender, replacing their previous
// one.
//
// Own decision only: the seat is taken from the authenticated principal and
// never from the request, so there is no shape of this call that records one
// person's answer under another's name.
func (s *SenderOverrideStore) Set(ctx context.Context, address, decision string) (SenderOverride, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return SenderOverride{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return SenderOverride{}, apperrors.ErrPermissionDenied
	}
	if decision != OverrideBusiness && decision != OverrideKeepOut {
		return SenderOverride{}, &InvalidOverrideError{Reason: "decision is business or keep_out"}
	}
	folded := normalizeEmail(address)
	if folded == "" {
		return SenderOverride{}, &InvalidOverrideError{Reason: "give one email address"}
	}
	var out SenderOverride
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The kind the machine had reached, read BEFORE the write so the page
		// can say what was overruled. A person who sees only their own answer
		// cannot tell a correction from a preference they set months ago.
		kind, err := machineKindForTx(ctx, tx, actor.UserID, folded)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO capture_sender_override (user_id, address, decision, overruled_kind)
			VALUES ($1, $2, $3, NULLIF($4, ''))
			ON CONFLICT (user_id, address) DO UPDATE
			   SET decision = EXCLUDED.decision,
			       overruled_kind = COALESCE(EXCLUDED.overruled_kind, capture_sender_override.overruled_kind),
			       updated_at = now()
			RETURNING id, address, decision, coalesce(overruled_kind, '')`,
			actor.UserID, folded, decision, kind).
			Scan(&out.ID, &out.Address, &out.Decision, &out.OverruledKind); err != nil {
			return fmt.Errorf("capture: recording a sender decision: %w", err)
		}
		// Audit-only, like the exclusion list beside it: this is capture
		// configuration, and the closed event catalog carries no type for it.
		// The object spelled as a literal, not through the constant: the audit
		// gate reads call sites to tell an entity named at runtime from one
		// named in the source, and a constant is something it cannot resolve.
		_, err = storekit.AuditEvent(ctx, tx, "update", "capture_settings", storekit.MustWorkspace(ctx),
			map[string]any{"decision": decision, "overruled_kind": kind})
		return err
	})
	return out, err
}

// List answers the caller's own decisions.
//
// Own rows only, and there is no id that reaches a colleague's: whose senders a
// person keeps out is itself private, and an admin view of it would be a list
// of somebody's private correspondents.
func (s *SenderOverrideStore) List(ctx context.Context) ([]SenderOverride, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []SenderOverride
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, address, decision, coalesce(overruled_kind, '')
			  FROM capture_sender_override
			 WHERE user_id = $1
			 ORDER BY created_at DESC`, actor.UserID)
		if err != nil {
			return fmt.Errorf("capture: listing sender decisions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var o SenderOverride
			if err := rows.Scan(&o.ID, &o.Address, &o.Decision, &o.OverruledKind); err != nil {
				return fmt.Errorf("capture: listing sender decisions: %w", err)
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// Remove withdraws a decision, handing the sender back to the machine.
func (s *SenderOverrideStore) Remove(ctx context.Context, address string) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return apperrors.ErrPermissionDenied
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM capture_sender_override WHERE user_id = $1 AND address = $2`,
			actor.UserID, normalizeEmail(address))
		if err != nil {
			return fmt.Errorf("capture: withdrawing a sender decision: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Not there and not yours are one answer, so an address probe
			// learns nothing about whose decisions exist.
			return apperrors.ErrNotFound
		}
		// Spelled as a literal for the audit gate; see Set.
		_, err = storekit.AuditEvent(ctx, tx, "update", "capture_settings", storekit.MustWorkspace(ctx),
			map[string]any{"decision": "withdrawn"})
		return err
	})
}

// OverrideForTx answers what this seat decided about a sender, or "" when they
// have not.
//
// THE VERDICT ENGINE CONSULTS THIS FIRST and never writes over it. A machine
// that could overturn a person would make every correction temporary, and the
// owner would have no way to tell a fresh mistake from one they already fixed.
func OverrideForTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (string, error) {
	folded := normalizeEmail(address)
	if user == ids.Nil || folded == "" {
		return "", nil
	}
	var decision string
	err := tx.QueryRow(ctx, `
		SELECT decision FROM capture_sender_override
		 WHERE user_id = $1 AND address = $2`, user, folded).Scan(&decision)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("capture: reading a seat's decision about a sender: %w", err)
	}
	return decision, nil
}

// machineKindForTx reads the kind the verdict engine last reached for this
// sender, so a recorded override can say what it overruled.
func machineKindForTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (string, error) {
	var kind string
	err := tx.QueryRow(ctx, `
		SELECT coalesce(kind, '') FROM capture_pending_counterparty
		 WHERE email = $1 AND owner_id = $2 AND kind IS NOT NULL
		 ORDER BY updated_at DESC LIMIT 1`, address, user).Scan(&kind)
	if err != nil {
		if err == pgx.ErrNoRows {
			// The engine never reached this sender. A person may still decide
			// about them — an address they know is coming, say — and the page
			// then shows a decision that overruled nothing.
			return "", nil
		}
		return "", fmt.Errorf("capture: reading what the machine decided about a sender: %w", err)
	}
	return kind, nil
}

// InvalidOverrideError is a malformed decision; it answers 422 naming the field.
type InvalidOverrideError struct{ Reason string }

func (e *InvalidOverrideError) Error() string { return "capture sender decision: " + e.Reason }

// FieldFault maps the refusal onto the wire.
func (e *InvalidOverrideError) FieldFault() (field, code, message string) {
	return "decision", "invalid_sender_decision", e.Reason
}
