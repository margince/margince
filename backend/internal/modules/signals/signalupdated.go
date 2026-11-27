// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// resolvedByOf is the user an outcome names. An agent's outcome names none, and
// its captured_by says which agent, so it never reads as the user's own decision.
func resolvedByOf(actor principal.Principal) *ids.UUID {
	if actor.Type == principal.PrincipalAgent {
		return nil
	}
	return storekit.UUIDOrNil(actor.UserID)
}

// emitSignalUpdated stages signal.updated beside the audit row. A subscriber
// sees a correction. A signal with no owner names the member who changed it.
// With no owner and no member id, no event is staged. The audit row still
// records the correction, and a zero id would name nobody.
func emitSignalUpdated(ctx context.Context, tx pgx.Tx, auditID, id, actor ids.UUID) error {
	var owner *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM signal WHERE id = $1`, id).Scan(&owner); err != nil {
		return fmt.Errorf("read signal owner: %w", err)
	}
	named, ok := correctionOwner(owner, actor)
	if !ok {
		return nil
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventSignalUpdated{SignalId: openapi_types.UUID(id), OwnerId: openapi_types.UUID(named)}); err != nil {
		return fmt.Errorf("emit signal.updated: %w", err)
	}
	return nil
}

// correctionOwner names who a signal.updated event reports: the owner, else the
// acting member. It reports false when neither is a real id.
func correctionOwner(owner *ids.UUID, actor ids.UUID) (ids.UUID, bool) {
	if owner != nil && *owner != (ids.UUID{}) {
		return *owner, true
	}
	return actor, actor != (ids.UUID{})
}
