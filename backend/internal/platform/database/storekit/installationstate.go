// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// InstallationStateKey names one row of installation_state: the bookkeeping of
// one background pass. A workspace reset clears the table, so a pass starts over.
type InstallationStateKey string

// The keys are listed together, so a new pass sees which rows are taken.
const (
	StateActivityClockRefoldMark     InstallationStateKey = "activity_clock_refold_mark"
	StateActivityWaitingHorizon      InstallationStateKey = "activity_waiting_horizon"
	StateCaptureAnsweredSweepCursor  InstallationStateKey = "capture_answered_sweep_cursor"
	StateCapturePartyAdmissionCursor InstallationStateKey = "capture_party_admission_cursor"
	StateDealAuthorshipSweepCursor   InstallationStateKey = "deal_authorship_sweep_cursor"
)

// InstallationStateTx reads the row of key into the shape its writer stores.
// found is false when the pass has stored nothing yet.
func InstallationStateTx[T any](ctx context.Context, tx pgx.Tx, key InstallationStateKey) (state T, found bool, err error) {
	return readInstallationState[T](ctx, tx, key, `SELECT value FROM installation_state WHERE key = $1`)
}

// LockInstallationStateTx reads the row and holds its lock until the
// transaction ends, so two runs of one pass take turns. It stores initial
// first when the row is absent, because only a stored row can be locked.
func LockInstallationStateTx[T any](ctx context.Context, tx pgx.Tx, key InstallationStateKey, initial T) (T, error) {
	var none T
	encoded, err := json.Marshal(initial)
	if err != nil {
		return none, fmt.Errorf("storekit: encoding the %s state: %w", key, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO installation_state (key, value) VALUES ($1, $2::jsonb)
		ON CONFLICT (key) DO NOTHING`, string(key), string(encoded)); err != nil {
		return none, fmt.Errorf("storekit: preparing the %s state: %w", key, err)
	}
	state, found, err := readInstallationState[T](ctx, tx, key,
		`SELECT value FROM installation_state WHERE key = $1 FOR UPDATE`)
	if err != nil {
		return none, err
	}
	if !found {
		return none, fmt.Errorf("storekit: the %s state was removed while it was being locked", key)
	}
	return state, nil
}

// StoreInstallationStateTx replaces the row of key with state.
func StoreInstallationStateTx[T any](ctx context.Context, tx pgx.Tx, key InstallationStateKey, state T) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("storekit: encoding the %s state: %w", key, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO installation_state (key, value, updated_at) VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		string(key), string(encoded)); err != nil {
		return fmt.Errorf("storekit: storing the %s state: %w", key, err)
	}
	return nil
}

func readInstallationState[T any](ctx context.Context, tx pgx.Tx, key InstallationStateKey, query string) (state T, found bool, err error) {
	var raw []byte
	if err := tx.QueryRow(ctx, query, string(key)).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return state, false, nil
		}
		return state, false, fmt.Errorf("storekit: reading the %s state: %w", key, err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, false, fmt.Errorf("storekit: decoding the %s state: %w", key, err)
	}
	return state, true, nil
}
