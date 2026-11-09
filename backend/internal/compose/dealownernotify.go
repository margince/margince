// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Deal stores have no job client; this inserter uses the caller's transaction.
var ownerChangeMail = sync.OnceValues(func() (*jobs.Runner, error) {
	return jobs.NewTxInserter(slog.Default())
})

// followDealOwnerAndTell hands a deal's pending requests to its new owner and
// queues their announcements in the deal writer's transaction. No second
// approval event is emitted for an existing request.
func followDealOwnerAndTell(ctx context.Context, tx pgx.Tx, dealID ids.UUID, from, to *ids.UUID) error {
	done, err := approvals.MoveDealRequestsInTx(ctx, tx, dealID, from, to)
	if err != nil {
		return err
	}
	store := notices.NewStore(nil)
	for _, withdrawn := range done.Withdrawn {
		if _, err := store.Retract(ctx, tx, approvalNoticeDedupeKey(withdrawn.UUID)); err != nil {
			return err
		}
	}
	if to == nil || len(done.Moved) == 0 {
		return nil
	}
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: a deal's owner changed outside a bound workspace")
	}
	queue, err := ownerChangeMail()
	if err != nil {
		return err
	}
	for _, request := range done.Moved {
		if _, err := store.Retract(ctx, tx, approvalNoticeDedupeKey(request.ID.UUID)); err != nil {
			return err
		}
		if err := queue.EnqueueTx(ctx, tx, ApprovalOwnerNoticeArgs{Workspace: wsID, ApprovalID: request.ID.UUID, DeciderID: *to}, nil); err != nil {
			return err
		}
	}
	return nil
}

// tellNewDecider gives the new owner a line about each request they may
// decide, and takes the former holders' lines back. The check is the staging
// announcement's, asked under the new owner's own authority.
func tellNewDecider(ctx context.Context, tx pgx.Tx, store *notices.Store, decider ids.UUID, moved []ids.ApprovalID, clock func() time.Time) error {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: a deal's owner changed outside a bound workspace")
	}
	seatCtx, live, err := deciderContextTx(ctx, tx, decider)
	if err != nil {
		return err
	}
	mail, err := ownerChangeMail()
	if err != nil {
		return err
	}
	if !live {
		return nil
	}
	for _, id := range moved {
		request, parts, metadata, err := approvals.PendingNoticeTx(seatCtx, tx, id, clock)
		if err != nil {
			return err
		}
		if request == nil {
			continue
		}
		var target notices.Target
		if request.TargetID != nil {
			target = notices.Target{Type: request.TargetType, ID: *request.TargetID}
		}
		line := approvalNoticeFor(decider, request.ID, request.Summary, parts, target, metadata)
		if err := recordApprovalNotice(ctx, tx, store.HandOverTx, mail, wsID, line); err != nil {
			return err
		}
	}
	return nil
}

// deciderContextTx holds the recipient's current authority until commit.
func deciderContextTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (context.Context, bool, error) {
	rbac, seatType, err := identity.EffectiveAuthorityTx(ctx, tx, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          principal.HumanIDPrefix + seat.String(),
		UserID:      seat,
		SeatType:    seatType,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	}), true, nil
}
