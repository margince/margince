// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	StateStakeholderMailBackfill     InstallationStateKey = "stakeholder_mail_backfill"
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

// The installation's own facts. The migration that made them copied each from
// the workspace table, and setup writes them when it creates the company.
const (
	StateStoredKeyWorkspace  InstallationStateKey = "stored_key_workspace"
	StateInstallationTrial   InstallationStateKey = "installation_trial"
	StateArchivedPredecessor InstallationStateKey = "archived_predecessor"
)

// StoredKeyWorkspace is the id that object-store keys and sealed-secret scopes
// were written under. Data already stored names it, so it never changes.
type StoredKeyWorkspace struct {
	ID ids.UUID `json:"id"`
}

// InstallationTrial is when the installation's first company was created, the
// day its trial counts from.
type InstallationTrial struct {
	StartedAt time.Time `json:"started_at"`
}

// ArchivedPredecessor counts the companies an earlier install archived into
// this one.
type ArchivedPredecessor struct {
	ArchivedCompanies int `json:"archived_companies"`
}

// Execer is what a fixture connection and a transaction both offer.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// RecordInstallationTx writes the facts of a newly created company. Both rows
// keep an earlier value: the stored-key id must not move once data is stored
// under it, and an archived company never starts a second trial.
func RecordInstallationTx(ctx context.Context, tx Execer, company ids.UUID, createdAt time.Time) error {
	for key, state := range map[InstallationStateKey]any{
		StateStoredKeyWorkspace: StoredKeyWorkspace{ID: company},
		StateInstallationTrial:  InstallationTrial{StartedAt: createdAt},
	} {
		encoded, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("storekit: encoding the %s state: %w", key, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO installation_state (key, value) VALUES ($1, $2::jsonb)
			ON CONFLICT (key) DO NOTHING`, string(key), string(encoded)); err != nil {
			return fmt.Errorf("storekit: recording the %s state: %w", key, err)
		}
	}
	return nil
}

// ErrNoStoredKeyWorkspace means the installation has no company yet, so nothing
// is stored under a workspace id.
var ErrNoStoredKeyWorkspace = errors.New("storekit: the installation has no stored-key workspace yet")

// StoredKeyWorkspaceTx reads the id stored keys are scoped by.
func StoredKeyWorkspaceTx(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	state, found, err := InstallationStateTx[StoredKeyWorkspace](ctx, tx, StateStoredKeyWorkspace)
	if err != nil {
		return ids.UUID{}, err
	}
	if !found {
		return ids.UUID{}, ErrNoStoredKeyWorkspace
	}
	return state.ID, nil
}

// KeptInstallationFacts is the part of installation_state a reset must not
// lose. A company is cleared, the installation is not.
type KeptInstallationFacts struct {
	company ids.UUID
	trial   time.Time
	held    bool
}

// KeptInstallationFactsTx reads the facts before a sweep clears the table.
func KeptInstallationFactsTx(ctx context.Context, tx pgx.Tx) (KeptInstallationFacts, error) {
	company, err := StoredKeyWorkspaceTx(ctx, tx)
	if errors.Is(err, ErrNoStoredKeyWorkspace) {
		return KeptInstallationFacts{}, nil
	}
	if err != nil {
		return KeptInstallationFacts{}, err
	}
	trial, found, err := InstallationStateTx[InstallationTrial](ctx, tx, StateInstallationTrial)
	if err != nil {
		return KeptInstallationFacts{}, err
	}
	if !found {
		return KeptInstallationFacts{}, errors.New("storekit: the installation has a stored-key workspace and no trial start")
	}
	return KeptInstallationFacts{company: company, trial: trial.StartedAt, held: true}, nil
}

// RestoreTx writes the facts back after the sweep.
func (k KeptInstallationFacts) RestoreTx(ctx context.Context, tx pgx.Tx) error {
	if !k.held {
		return nil
	}
	return RecordInstallationTx(ctx, tx, k.company, k.trial)
}

// ReadStoredKeyWorkspace reads the id stored keys are scoped by, for a caller that
// holds a pool and no transaction.
func ReadStoredKeyWorkspace(ctx context.Context, pool *pgxpool.Pool) (ids.UUID, error) {
	var company ids.UUID
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		company, err = StoredKeyWorkspaceTx(ctx, tx)
		return err
	})
	return company, err
}
