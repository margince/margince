// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Settling a claim: saying the promise was kept, or that it no longer stands.
//
// The status column has carried open/done/dismissed since the table existed and
// every reader filters on `open` — the commitments card, the project rollup, the
// worklist lane. Nothing wrote the other two. A promise could be extracted from
// a conversation and shown to the rep who made it every morning, with no way to
// say they had kept it; the only writes of 'done' in the tree were test
// fixtures hand-inserting what production could not.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// claimStatusOpen is the state a claim is settled OUT of. A claim already
// settled is not settled again, and one settled the other way is a conflict
// rather than an overwrite.
const claimStatusOpen = "open"

// claimIDKey names the claim inside a contact's audit image. The trail's entity
// is the CONTACT — a claim is a fact about one — so the row has to say which of
// their claims moved.
const claimIDKey = "claim_id"

// SettleConversationClaim records that a claim finished, and how.
//
// Gated on `contact` UPDATE and the contact's own writability, the same pair
// RecordConversationClaim holds: a claim is a fact about a contact, and settling
// one writes to that contact's record as much as recording it did.
//
// Human-only in the STORE as well as the router table, for the reason the
// recording path states: a store relying on the routing declaration alone is one
// x-mcp-tool away from being agent-reachable without anybody revisiting it. And
// here the stakes are the reason to keep it — whether a human kept their word is
// not a thing to let an extractor decide about itself.
func (s *Store) SettleConversationClaim(ctx context.Context, claimID ids.UUID, outcome string) error {
	if outcome != string(crmcontracts.SettleClaimRequestOutcomeDone) &&
		outcome != string(crmcontracts.SettleClaimRequestOutcomeDismissed) {
		return httperr.Validation("outcome", "invalid",
			"a claim settles as done or dismissed")
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		// LOCKED before the status is read. Without FOR UPDATE two concurrent
		// settles both read `open`, both pass the transition check, and the
		// second overwrites the first — 204 to both callers, and the loser's
		// audit row claims it moved a claim that was already settled. The
		// conflict below only refuses a settlement it can SEE.
		if _, err := storekit.LockRow(ctx, tx, "conversation_claim", claimID, storekit.LiveOnly); err != nil {
			return err
		}
		var contactID, activityID ids.UUID
		var current string
		if err := tx.QueryRow(ctx, `
			SELECT contact_id, source_activity_id, status FROM conversation_claim
			WHERE id = $1`, claimID).Scan(&contactID, &activityID, &current); err != nil {
			return fmt.Errorf("read the claim to settle: %w", err)
		}
		// The contact's gate, not the claim's: a claim is reachable through the
		// contact it is about, so a caller who may not write that contact may not
		// settle their promises. A contact outside the caller's scope reads as
		// absent, which is what keeps the claim's existence from leaking.
		if err := auth.EnsureWritableLive(ctx, tx, "contact", contactID); err != nil {
			return err
		}
		// And the message the claim was read from, the same gate the recording
		// path holds. A claim quotes words, and its evidence can be narrowed or
		// archived after it was written — a caller who once saw the conversation
		// must not go on settling promises read out of it, and the 204-vs-409
		// answer would report the claim's current status to somebody who may no
		// longer read what it rests on.
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activityID); err != nil {
			return err
		}
		if current == outcome {
			// The caller's goal state already holds. Settling twice is one
			// settlement, so this is success and writes nothing — a second audit
			// row would say a contact's record changed when it did not.
			return nil
		}
		if current != claimStatusOpen {
			return fmt.Errorf("this claim was already settled as %s: %w", current, apperrors.ErrConflict)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE conversation_claim SET status = $2 WHERE id = $1`,
			claimID, outcome); err != nil {
			return fmt.Errorf("settle the claim: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "contact", contactID,
			map[string]any{"claim_status": current, claimIDKey: claimID.String()},
			map[string]any{"claim_status": outcome, claimIDKey: claimID.String()})
		if err != nil {
			return fmt.Errorf("audit the settlement: %w", err)
		}
		// The EXISTING changed event, not a settled one of its own. Its
		// contract already names this case — "a human typed over the machine,
		// or dismissed a claim outright" — and carries the two facts a
		// subscriber needs, so a second event would be a second way to learn
		// the same thing.
		return storekit.EmitEvent(ctx, tx, auditID, contactID,
			crmcontracts.PublicEventConversationClaimChanged{
				ClaimId: openapi_types.UUID(claimID),
				Status:  outcome,
			})
	})
}

// SettleConversationClaim implements (POST /claims/{id}/settle).
//
// 204 and no body: the caller told the server how a claim finished, and there is
// nothing to read back that they did not just send. The lanes it leaves are
// re-read by the surfaces that list open claims.
func (h Handlers) SettleConversationClaim(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.SettleClaimRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.SettleConversationClaim(r.Context(), ids.UUID(id), string(req.Outcome)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
