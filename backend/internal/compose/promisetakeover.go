// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A colleague takes over a promise nobody was asked to keep. Such a claim
// stays open on its contact with no task (CommitmentPromiserUnassignable).
// Taking it over is the explicit act that names the new owner. The task goes
// through the commitment's one task writer, keyed on the promise's locator.
// A later reading of the same conversation then finds it and writes no second.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promiseTakeOverHandlers serves POST /claims/{id}/take-over.
type promiseTakeOverHandlers struct {
	pool     *pgxpool.Pool
	claims   *contacts.Store
	tasks    *activities.Store
	approval *approvals.Service
	now      func() time.Time
}

func newPromiseTakeOverHandlers(pool *pgxpool.Pool) promiseTakeOverHandlers {
	db := InstallationDB(pool)
	return promiseTakeOverHandlers{
		pool: pool, claims: contacts.NewStore(db), tasks: activities.NewStore(db),
		approval: NewApprovalsService(db), now: time.Now,
	}
}

// TakeOverConversationClaim implements POST /claims/{id}/take-over.
func (h promiseTakeOverHandlers) TakeOverConversationClaim(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.TakeOver(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// TakeOver makes the calling colleague the owner of the promise behind one
// claim. The same colleague pressing again gets the first answer back and
// nothing is written.
func (h promiseTakeOverHandlers) TakeOver(ctx context.Context, claimID ids.UUID) (crmcontracts.PromiseTakeOver, error) {
	taker, err := takingColleague(ctx)
	if err != nil {
		return crmcontracts.PromiseTakeOver{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.PromiseTakeOver{}, err
	}
	var out crmcontracts.PromiseTakeOver
	err = database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		promise, err := h.claims.PromiseToTakeTx(ctx, tx, claimID)
		if err != nil {
			return err
		}
		if !takenOverBy(promise, taker) {
			if err := h.write(ctx, tx, promise, taker); err != nil {
				return err
			}
			if promise, err = h.claims.PromiseToTakeTx(ctx, tx, claimID); err != nil {
				return err
			}
		}
		out, err = takenOver(promise)
		return err
	})
	return out, err
}

// write takes the promise over for taker, or says why it is not theirs to take.
func (h promiseTakeOverHandlers) write(ctx context.Context, tx pgx.Tx, promise contacts.PromiseToTake, taker ids.UUID) error {
	if err := refuseHeldPromise(promise); err != nil {
		return err
	}
	if err := lockPromiseSource(ctx, tx, promise.Source); err != nil {
		return err
	}
	if err := h.refuseOfferedPromise(ctx, tx, promise); err != nil {
		return err
	}
	// Locked first, so the audience read below and the reader check are of
	// one source that no concurrent narrowing can change before commit.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity WHERE id = $1 FOR SHARE`, promise.Source); err != nil {
		return fmt.Errorf("compose: locking the promise's source: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, promise.Source); err != nil {
		return err
	}
	privateTo, err := takenOverAudience(ctx, tx, promise.Source, taker)
	if err != nil {
		return err
	}
	seat, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](taker))
	if err != nil {
		return err
	}
	if seat == activities.SeatCannotHoldWork {
		return fmt.Errorf("your seat cannot be given tasks, so it cannot take over a promise: %w", apperrors.ErrPermissionDenied)
	}
	task, err := writeCommitmentTask(ctx, tx, h.tasks, h.claims, commitmentTask{
		Locator: promise.Locator, Summary: promise.Body, Body: takenOverBody(promise),
		SourceActivityID: promise.Source, DueAt: promise.DueAt, Assignee: taker, PrivateTo: privateTo,
		Links:  []activities.ActivityLinkInput{{EntityType: string(recordTypeContact), EntityID: promise.Contact}},
		Origin: activities.OriginHuman,
	})
	if err != nil {
		return err
	}
	return h.claims.RecordPromiseTakenTx(ctx, tx, promise, task, taker, h.now().UTC())
}

// takenOverAudience is who may read the task. The task carries the promise's
// words and the source's name. So when the source is narrower than every
// colleague, only the taker reads it. Mail private to another colleague is
// refused.
func takenOverAudience(ctx context.Context, tx pgx.Tx, source, taker ids.UUID) (*ids.UUID, error) {
	privateTo, err := sourcePrivateToNow(ctx, tx, source, nil)
	if err != nil {
		return nil, err
	}
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, source, privateTo, taker)
	if err != nil {
		return nil, err
	}
	if !may {
		return nil, fmt.Errorf("this promise is in mail only another colleague may read, so only they can take it over: %w",
			apperrors.ErrPermissionDenied)
	}
	var audience string
	if err := tx.QueryRow(ctx, `SELECT audience FROM activity WHERE id = $1`, source).Scan(&audience); err != nil {
		return nil, fmt.Errorf("compose: reading who may read the promise's source: %w", err)
	}
	if audience != string(crmcontracts.ActivityAudienceWorkspace) {
		return &taker, nil
	}
	return privateTo, nil
}

// takingColleague is the signed-in colleague behind the call. An agent, a
// connector or one of the product's own passes is refused. Taking over a
// promise is a colleague's word that they will keep it.
func takingColleague(ctx context.Context) (ids.UUID, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.Nil, fmt.Errorf("only a signed-in colleague can take over a promise: %w", apperrors.ErrPermissionDenied)
	}
	return actor.UserID, nil
}

// refuseHeldPromise answers a promise that already has an owner, or is no
// longer one to keep, with a conflict that says which.
func refuseHeldPromise(promise contacts.PromiseToTake) error {
	switch {
	case promise.TakenBy != nil && promise.TakenAt != nil:
		who := "A colleague"
		if promise.TakerName != nil {
			who = *promise.TakerName
		}
		return fmt.Errorf("%s took this promise over on %s: %w", who, promise.TakenAt.Format(time.DateOnly), apperrors.ErrConflict)
	case promise.Kind != claimKindOurs:
		return fmt.Errorf("only a promise one of us made can be taken over: %w", apperrors.ErrConflict)
	case promise.Status != string(crmcontracts.ConversationClaimStatusOpen):
		return fmt.Errorf("this promise is already settled as %s: %w", promise.Status, apperrors.ErrConflict)
	case promise.Task != nil || len(promise.Claims) == 0:
		return fmt.Errorf("this promise is already somebody's task: %w", apperrors.ErrConflict)
	case promise.NeedsReview || !promise.Current:
		return fmt.Errorf("this promise is disputed or no longer current, so it cannot be taken over: %w", apperrors.ErrConflict)
	}
	return nil
}

// refuseOfferedPromise refuses a promise that already has a task under its
// locator. It also refuses one its maker was asked about and has not answered,
// or accepted with the task not yet written. Each is somebody's already.
func (h promiseTakeOverHandlers) refuseOfferedPromise(ctx context.Context, tx pgx.Tx, promise contacts.PromiseToTake) error {
	written, err := h.tasks.CommitmentTaskWritten(ctx, tx, promise.Locator)
	if err != nil {
		return err
	}
	if written {
		return fmt.Errorf("this promise is already somebody's task: %w", apperrors.ErrConflict)
	}
	offers, err := h.approval.OffersForTx(ctx, tx, CommitmentTaskKind, promise.Source)
	if err != nil {
		return err
	}
	for _, offer := range offers {
		if !offer.Pending() && !offer.Approved() {
			continue
		}
		var proposal CommitmentTaskProposal
		if err := json.Unmarshal(offer.Change, &proposal); err != nil {
			return fmt.Errorf("compose: unmarshal a commitment proposal: %w", err)
		}
		if proposal.Locator == promise.Locator {
			return fmt.Errorf("this promise was put to the colleague who made it: %w", apperrors.ErrConflict)
		}
	}
	return nil
}

// takenOverBody says where the task came from, in words the taker can check.
func takenOverBody(promise contacts.PromiseToTake) string {
	if promise.SourceLabel != nil && *promise.SourceLabel != "" {
		return fmt.Sprintf("Promise taken over from %q. Nobody was asked to keep it.", *promise.SourceLabel)
	}
	return "Promise taken over from a captured conversation. Nobody was asked to keep it."
}

// takenOverBy reports whether taker already holds this promise as their task.
func takenOverBy(promise contacts.PromiseToTake, taker ids.UUID) bool {
	return promise.TakenBy != nil && *promise.TakenBy == taker && promise.Task != nil && promise.TakenAt != nil
}

func takenOver(promise contacts.PromiseToTake) (crmcontracts.PromiseTakeOver, error) {
	if promise.TakenBy == nil || !takenOverBy(promise, *promise.TakenBy) {
		return crmcontracts.PromiseTakeOver{}, errors.New("compose: a promise taken over carries no taker")
	}
	return crmcontracts.PromiseTakeOver{
		ClaimId:        openapi_types.UUID(promise.Claim),
		TaskActivityId: openapi_types.UUID(*promise.Task),
		TakenByUserId:  openapi_types.UUID(*promise.TakenBy),
		TakenByName:    promise.TakerName,
		TakenAt:        *promise.TakenAt,
	}, nil
}
