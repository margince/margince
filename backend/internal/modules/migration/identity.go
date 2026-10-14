// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Binding is a landed identity and the state of the run that landed it.
type Binding struct {
	NativeID  ids.UUID
	RunStatus string
}

// LookupIdentity resolves an external id to the native row a previous (or the
// current) run landed for it. Only the engine-owned map says "already
// imported". A row's own source columns are client-writable, so a caller could
// pre-plant a row under a source id and pass it off as landed.
func (s *RunStore) LookupIdentity(ctx context.Context, sourceSystem, object, externalID string) (ids.UUID, bool, error) {
	binding, found, err := s.LookupBinding(ctx, sourceSystem, object, externalID)
	return binding.NativeID, found, err
}

// LookupBinding is LookupIdentity plus the status of the run that made the
// binding, which decides whether the key may be taken over. A bound ref
// resolves like a created one.
func (s *RunStore) LookupBinding(ctx context.Context, sourceSystem, object, externalID string) (Binding, bool, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionRead); err != nil {
		return Binding{}, false, err
	}
	var binding Binding
	found := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			-- The workspace predicate is the lookup's own: tenant isolation
			SELECT m.native_id, r.status FROM import_record_map m
			  JOIN import_run r ON r.id = m.import_run_id
			WHERE m.source_system = $1 AND m.object = $2 AND m.external_id = $3`,
			sourceSystem, object, externalID).Scan(&binding.NativeID, &binding.RunStatus)
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

// RecordIdentity records the external→native identity a run just landed.
// Idempotent: a resumed run replaying its last page re-asserts the same
// pair rather than failing.
func (s *RunStore) RecordIdentity(ctx context.Context, runID RunID, sourceSystem, object, externalID string, nativeID ids.UUID) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionCreate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return recordIdentityInTx(ctx, tx, runID, identityRow{sourceSystem, object, externalID, nativeID, OriginCreated})
	})
}

// RecordIdentityTx is RecordIdentity in a transaction the caller opened. The
// writer that lands a native row maps it in the same commit. A crash then
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
	}
	if err != nil {
		return fmt.Errorf("migration: recording the %s %s identity: %w", row.object, row.externalID, err)
	}
	return nil
}

// IdentityPair is one source-record → native-record binding.
type IdentityPair struct {
	ExternalID string
	NativeID   ids.UUID
}

// RecordIdentities records many bindings in one statement, for the
// resume-time repair that adopts records a crashed attempt created but
// never got to map. Same conflict rule as RecordIdentity: an existing
// binding stands, so repairing twice is a no-op and a binding this run
// already holds is never overwritten.
func (s *RunStore) RecordIdentities(ctx context.Context, runID RunID, sourceSystem, object string, pairs []IdentityPair) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionCreate); err != nil {
		return err
	}
	if len(pairs) == 0 {
		return nil
	}
	externals := make([]string, len(pairs))
	natives := make([]ids.UUID, len(pairs))
	for i, p := range pairs {
		externals[i], natives[i] = p.ExternalID, p.NativeID
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO import_record_map (source_system, object, external_id, native_id, import_run_id)
			SELECT $1, $2, e, n, $5
			FROM unnest($3::text[], $4::uuid[]) AS t(e, n)
			ON CONFLICT (source_system, object, external_id) DO NOTHING`,
			sourceSystem, object, externals, natives, runID)
		if storekit.IsForeignKeyViolation(err) {
			return fmt.Errorf("import run %s: %w", runID, apperrors.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("migration: recording %d %s identities: %w", len(pairs), object, err)
		}
		return nil
	})
}
