// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Binding is a landed identity and the state of the run that landed it.
type Binding struct {
	RunID     RunID
	NativeID  ids.UUID
	RunStatus string
	Origin    Origin
}

// LookupBinding resolves an external id to the native row a run landed for it,
// with the status and origin of that run's identity.
// Only the engine-owned map says "already imported".
// A row's own source columns are client-writable, so a caller could pre-plant
// one and pass it off as landed.
func (s *RunStore) LookupBinding(ctx context.Context, sourceSystem, object, externalID string) (Binding, bool, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionRead); err != nil {
		return Binding{}, false, err
	}
	var binding Binding
	found := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			-- The workspace predicate is the lookup's own: tenant isolation
			SELECT m.native_id, r.status, m.origin, r.id FROM import_record_map m
			  JOIN import_run r ON r.id = m.import_run_id
			WHERE m.source_system = $1 AND m.object = $2 AND m.external_id = $3`,
			sourceSystem, object, externalID).Scan(&binding.NativeID, &binding.RunStatus, &binding.Origin, &binding.RunID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("migration: looking up the %s %s identity: %w", object, externalID, err)
		}
		found = true
		return nil
	})
	if err != nil {
		return Binding{}, false, err
	}
	return binding, found, nil
}

// Origin says how a run came to hold an identity. Undo archives only what the
// run created; a bound record existed before the run and only its ref is new.
type Origin string

const (
	// OriginCreated is a record the run created.
	OriginCreated Origin = "created"
	// OriginBound is an existing record a row named by id, so its ref resolves.
	OriginBound Origin = "bound"
)

// RecordIdentityTx records the external→native identity a run just landed, in
// a transaction the caller opened. Idempotent: a resumed run replaying its last
// page re-asserts the same pair. The writer that lands a native row maps it in the same commit. A crash then
// cannot leave a record the resume has no name for.
func (s *RunStore) RecordIdentityTx(
	ctx context.Context, tx pgx.Tx, runID RunID, sourceSystem, object, externalID string, nativeID ids.UUID, origin Origin,
) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionCreate); err != nil {
		return err
	}
	return recordIdentityInTx(ctx, tx, runID, identityRow{sourceSystem, object, externalID, nativeID, origin})
}

// identityRow is one import_record_map row a run writes.
type identityRow struct {
	sourceSystem, object, externalID string
	nativeID                         ids.UUID
	origin                           Origin
}

// recordIdentityInTx is the statement both entry points run.
func recordIdentityInTx(ctx context.Context, tx pgx.Tx, runID RunID, row identityRow) error {
	// A finished undo frees its keys; a paused one still pages them by offset.
	if _, err := tx.Exec(ctx, `
		DELETE FROM import_record_map m
		 USING import_run r
		 WHERE r.id = m.import_run_id AND r.status = $4
		   AND m.source_system = $1 AND m.object = $2 AND m.external_id = $3`,
		row.sourceSystem, row.object, row.externalID, StatusUndone); err != nil {
		return fmt.Errorf("migration: releasing the %s %s identity: %w", row.object, row.externalID, err)
	}
	// The statement resolves the run instead of trusting the argument. A run
	// that does not exist makes the INSERT write nothing, and the caller gets
	// the not-found below, never a foreign-key error naming a table.
	tag, err := tx.Exec(ctx, `
		INSERT INTO import_record_map (source_system, object, external_id, native_id, import_run_id, origin)
		SELECT $1, $2, $3, $4, r.id, $6
		  FROM import_run r
		 WHERE r.id = $5
		ON CONFLICT (source_system, object, external_id) DO NOTHING`,
		row.sourceSystem, row.object, row.externalID, row.nativeID, runID, string(row.origin))
	if err == nil && tag.RowsAffected() == 0 {
		// Either the run does not exist, or the identity is already mapped.
		// Distinguished here rather than guessed: a replay must stay a no-op,
		// and only a missing run is an error.
		var exists bool
		if probeErr := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM import_run WHERE id = $1)`,
			runID).Scan(&exists); probeErr != nil {
			return fmt.Errorf("migration: resolving the import run %s: %w", runID, probeErr)
		}
		if !exists {
			return fmt.Errorf("import run %s: %w", runID, apperrors.ErrNotFound)
		}
		// A concurrent run may have mapped the identity to another record
		// since the caller's pre-check; a replay of the same record is a no-op.
		var incumbent ids.UUID
		if probeErr := tx.QueryRow(ctx, `
			SELECT native_id FROM import_record_map
			 WHERE source_system = $1 AND object = $2 AND external_id = $3`,
			row.sourceSystem, row.object, row.externalID).Scan(&incumbent); probeErr != nil {
			return fmt.Errorf("migration: resolving the %s %s identity: %w", row.object, row.externalID, probeErr)
		}
		if incumbent != row.nativeID {
			return fmt.Errorf("migration: %s %s already maps to another record", row.object, row.externalID)
		}
	}
	if err != nil {
		return fmt.Errorf("migration: recording the %s %s identity: %w", row.object, row.externalID, err)
	}
	return nil
}
