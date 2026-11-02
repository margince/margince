// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingCopyReason is the archive reason a retired meeting copy records.
const meetingCopyReason = "meeting_duplicate"

// RetireMeetingCopyTx archives a meeting stored twice and moves its identities
// to the row that stays. The caller has proved the two rows are one meeting
// and that retiring the copy loses nothing. Only a system job or a calendar
// sync may call it: the fold acts on a colleague's row, which no human grant
// covers. The audit row names the survivor, so unarchiving undoes it.
func (s *Store) RetireMeetingCopyTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || (actor.Type != principal.PrincipalSystem && actor.Type != principal.PrincipalConnector) {
		return fmt.Errorf("activities: only a system job or a calendar sync retires a meeting copy: %w", apperrors.ErrPermissionDenied)
	}
	if err := TransferIdentities(ctx, tx, duplicate, survivor); err != nil {
		return err
	}
	p := storekit.NewPatch()
	p.Set("archived_at", nil, s.now().UTC())
	lock, err := storekit.LockRow(ctx, tx, "activity", duplicate.UUID, storekit.LiveOnly)
	if err != nil {
		return fmt.Errorf("activities: locking a meeting copy: %w", err)
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("activities: archiving a meeting copy: %w", err)
	}
	evidence := map[string]any{archiveReasonKey: meetingCopyReason, "survivor": survivor.String()}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "archive", "activity", duplicate.UUID, p.Before(), p.After(), evidence)
	if err != nil {
		return err
	}
	archived := crmcontracts.PublicEventActivityArchived{}
	archived.Imported, archived.ImportRunId = storekit.ImportMark(ctx)
	return storekit.EmitEvent(ctx, tx, auditID, duplicate.UUID, archived)
}
