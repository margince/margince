// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func currentConsentState(ctx context.Context, tx pgx.Tx, sub subject, purpose ids.PurposeID) (string, error) {
	var current string
	err := tx.QueryRow(ctx, `SELECT state FROM contact_consent WHERE `+sub.column+` = $1 AND purpose_id = $2 FOR UPDATE`, sub.id, purpose).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return current, err
}

// PreviewInitialGrant preserves existing decisions without recording proof or events.
func (s *Store) PreviewInitialGrant(ctx context.Context, in RecordInput) (State, error) {
	if !in.NeverOverrideExisting || in.NewState != string(StateGranted) {
		return State{}, apperrors.ErrInvalidArgument
	}
	sub, state, err := admitRecord(ctx, in)
	if err != nil {
		return State{}, err
	}
	var out State
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		probe, err := rowProbeFor(state)
		if err != nil {
			return err
		}
		if err := probe(ctx, tx, sub.entityType, sub.id); err != nil {
			return err
		}
		key, doi, err := loadConsentPurpose(ctx, tx, in.PurposeID)
		if err != nil {
			return err
		}
		current, err := currentConsentState(ctx, tx, sub, in.PurposeID)
		if err != nil {
			return err
		}
		out = State{PurposeID: in.PurposeID, PurposeKey: key, State: current}
		if current != "" {
			return nil
		}
		if _, err := s.resolveDOIConfirmation(ctx, tx, in, sub, doi); err != nil {
			return err
		}
		out.State, out.Changed = in.NewState, true
		return nil
	})
	return out, err
}
