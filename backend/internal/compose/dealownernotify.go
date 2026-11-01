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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerChangeMail stages the email about a moved request. Deal stores are
// built without a job client, so this one holds no pool and only inserts
// through the deal writer's transaction.
var ownerChangeMail = sync.OnceValues(func() (*jobs.Runner, error) {
	return jobs.NewTxInserter(slog.Default())
})

// followDealOwnerAndTell hands a deal's pending requests to its new owner and
// tells the members concerned, in the deal writer's transaction. It writes no
// approval event, so webhook subscribers do not see a request twice.
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
	return tellNewDecider(ctx, tx, store, *to, done.Moved)
}

// tellNewDecider gives the new owner a line about each request they may
// decide, and takes the former holders' lines back. The check is the staging
// announcement's, asked under the new owner's own authority.
func tellNewDecider(ctx context.Context, tx pgx.Tx, store *notices.Store, decider ids.UUID, moved []approvals.MovedRequest) error {
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
	now := time.Now()
	for _, request := range moved {
		key := approvalNoticeDedupeKey(request.ID.UUID)
		decidable := false
		var parts *crmcontracts.ApprovalSummaryParts
		if live {
			if decidable, parts, err = approvals.PendingAskOfAtTx(seatCtx, tx, request.ID, now); err != nil {
				return err
			}
		}
		if !decidable {
			if _, err := store.Retract(ctx, tx, key); err != nil {
				return err
			}
			continue
		}
		var target notices.Target
		if request.TargetID != nil {
			target = notices.Target{Type: request.TargetType, ID: *request.TargetID}
		}
		line := approvalNoticeFor(decider, request.ID, request.Summary, parts, target)
		if err := recordApprovalNotice(ctx, tx, store.HandOverTx, mail, wsID, line); err != nil {
			return err
		}
	}
	return nil
}

// deciderContextTx binds the new owner's own authority, read in the deal
// writer's transaction. live is false for a member who is not live.
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
