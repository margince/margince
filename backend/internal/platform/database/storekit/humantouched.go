// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

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
