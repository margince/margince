// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// InspectMemberHandoffsTx separates retirement dependencies from visible work.
func InspectMemberHandoffsTx(ctx context.Context, tx pgx.Tx, member ids.UUID, receiver *principal.Principal) (workforce.MemberWorkSnapshot, error) {
	var snapshot workforce.MemberWorkSnapshot
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return snapshot, err
	}
	rows, err := tx.Query(ctx, `SELECT id,version FROM sdr_handoff WHERE assigned_to=@member AND status=@status ORDER BY id`, pgx.StrictNamedArgs{"member": member, handoffStatusKey: HandoffSubmitted})
	if err != nil {
		return snapshot, err
	}
	type pending struct {
		ID      ids.UUID
		Version int64
	}
	handoffs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[pending])
	if err != nil {
		return snapshot, err
	}
	snapshot.Owned = len(handoffs)
	for _, handoff := range handoffs {
		snapshot.Stamp = append(snapshot.Stamp, fmt.Sprintf("sdr_handoff:%s:%d", handoff.ID, handoff.Version))
		err := readableHandoffSubject(ctx, tx, handoff.ID)
		if err != nil {
			if handoffReassignmentRefused(err) {
				continue
			}
			return snapshot, err
		}
		if err := auth.Require(ctx, HandoffObject, principal.ActionRead); err != nil {
			continue
		}
		snapshot.Visible++
		if receiver == nil {
			continue
		}
		if err := auth.Require(ctx, HandoffObject, principal.ActionUpdate); err != nil {
			continue
		}
		if err := ensureHandoffSubjectWritable(ctx, tx, handoff.ID); err != nil {
			if handoffReassignmentRefused(err) {
				continue
			}
			return snapshot, err
		}
		if err := requireHandoffReceiver(ctx, tx, handoff.ID, *receiver); err != nil {
			if handoffReassignmentRefused(err) {
				continue
			}
			return snapshot, err
		}
		snapshot.Transferable++
	}
	return snapshot, nil
}
