// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AuditNotFromAnImport is the audit_log predicate for a row a user
// wrote, as against one an import run landed. A re-import's update carries the
// approving user as its actor but is the import's own write. It must not make
// an earlier run's undo keep the row. A bulk change's batch is not an
// import run and still counts. The undo's page check and HumanActedSince
// share it.
func AuditNotFromAnImport(alias string) string {
	return fmt.Sprintf(`(%[1]s.batch_id IS NULL OR NOT EXISTS (SELECT 1 FROM import_run ir WHERE ir.id = %[1]s.batch_id))`, alias)
}

// HumanActedSince reports whether a user or their agent has acted on one record
// after `at`. Acting is any such audit row on the entity, not only an update. A
// narrower filter would reverse the act of somebody who archived an imported
// row themselves.
//
// Held by: TestTheDealArchiveReAsksUntouchedInsideItsTransaction
// (backend/internal/compose/integration).
func HumanActedSince(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, at time.Time) (bool, error) {
	var touched bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM audit_log a
			 WHERE a.entity_type = $1 AND a.entity_id = $2
			   AND `+AuditForAUser("a")+` AND a.occurred_at > $3
			   AND `+AuditNotFromAnImport("a")+`)`,
		entityType, id, at).Scan(&touched); err != nil {
		return false, fmt.Errorf("checking whether a colleague has acted on this %s: %w", entityType, err)
	}
	return touched, nil
}

// MachineCreatedUntouched requires retained creation proof; missing audit history
// cannot authorize an automatic repair of work a colleague may have adopted. A
// record a user's agent created or touched is that user's work.
func MachineCreatedUntouched(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (bool, error) {
	var pristine bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_log made WHERE made.entity_type=@kind AND made.entity_id=@id AND made.action='create' AND made.actor_type IN ('system','agent') AND made.passport_id IS NULL)
 AND NOT EXISTS (SELECT 1 FROM audit_log worked WHERE worked.entity_type=@kind AND worked.entity_id=@id AND `+AuditForAUser("worked")+`)`, pgx.NamedArgs{"kind": entityType, "id": id}).Scan(&pristine)
	return pristine, err
}

// HumanTouchedError refuses an archive whose caller asked for it only while the
// record was untouched since an import landed it. It is a conflict, so an undo
// keeps the record and says why instead of stopping.
type HumanTouchedError struct {
	EntityType string
	EntityID   ids.UUID
}

func (e *HumanTouchedError) Error() string {
	return fmt.Sprintf("a colleague has changed this %s since it was imported, so it was left alone", e.EntityType)
}

// Is makes the refusal a conflict for errors.Is.
func (e *HumanTouchedError) Is(target error) bool { return target == apperrors.ErrConflict }

// ArchiveGuard is a precondition on an archive: the record must not have been
// worked on by a colleague since the import landed it.
type ArchiveGuard struct {
	since             time.Time
	releasesSourceKey bool
}

// ReleasingSourceKey makes the archive free the record's id in its source
// system, so the same file imports it again after an undo.
func ReleasingSourceKey() ArchiveGuard { return ArchiveGuard{releasesSourceKey: true} }

// ReleasesSourceKey reports whether any guard asks the archive to free the key.
func ReleasesSourceKey(guards ...ArchiveGuard) bool {
	for _, guard := range guards {
		if guard.releasesSourceKey {
			return true
		}
	}
	return false
}

// NotTouchedByHumanSince guards an archive against a record a user or their agent
// has acted on after `at`.
func NotTouchedByHumanSince(at time.Time) ArchiveGuard { return ArchiveGuard{since: at} }

// RefuseIfHumanActedSince asks each guard inside the caller's transaction, after
// the row's write check took the lock. A colleague's write then lands wholly
// before the question or wholly after the commit.
func RefuseIfHumanActedSince(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, guards ...ArchiveGuard) error {
	for _, guard := range guards {
		if guard.since.IsZero() {
			continue
		}
		touched, err := HumanActedSince(ctx, tx, entityType, id, guard.since)
		if err != nil {
			return err
		}
		if touched {
			return &HumanTouchedError{EntityType: entityType, EntityID: id}
		}
	}
	return nil
}
