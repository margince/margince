// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The share request a create raises when a colleague holds the address.
//
// contacts refuses the create and asks through its ShareRequestFiler; the
// request lives in the approvals queue and the share it releases is identity's
// record grant. No module imports another, so the three meet here.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	contactShareKind = approvalsubject.KindContactShare
	// contactShareActor stages the request, which the server files for the
	// requester. Staged under an agent working for the requester, a proposal
	// for a different member is refused (payloadNamesItsOwner).
	contactShareActor = "agent:" + contactShareKind
)

// contactShareRequests is the approvals queue behind contacts' share
// requests.
func contactShareRequests(pool *pgxpool.Pool) contacts.ShareRequests {
	return contactShareQueue{pool: pool}
}

type contactShareQueue struct{ pool *pgxpool.Pool }

// FollowOwnerTx withdraws the contact's requests that name another owner.
func (q contactShareQueue) FollowOwnerTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, to *ids.UUID) error {
	_, err := approvals.FollowContactOwnerInTx(ctx, tx, contactID.UUID, to)
	return err
}

// FileTx files one request per contact and requester. A repeat joins the open
// one; after the owner declined, nothing is filed and the answer is false.
//
// The approvals service is built per request, not with the queue: its preview
// reader composes the contacts provider, which composes this queue.
func (q contactShareQueue) FileTx(ctx context.Context, tx pgx.Tx, req contacts.ShareRequest) (bool, error) {
	svc := NewApprovalsService(InstallationDB(q.pool))
	body, err := json.Marshal(approvalsubject.ContactShare{
		ContactID: req.ContactID.UUID, OwnerID: req.HolderID, RequesterID: req.RequesterID,
	})
	if err != nil {
		return false, fmt.Errorf("compose: encoding the share request: %w", err)
	}
	identityKey, err := json.Marshal(map[string]ids.UUID{
		companionContactField: req.ContactID.UUID, "requester_id": req.RequesterID,
	})
	if err != nil {
		return false, fmt.Errorf("compose: encoding the share request's identity: %w", err)
	}
	summary, err := contactShareSummary(ctx, tx, req.RequesterID)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(body)
	_, open, err := svc.StageUnlessDeclinedTx(principal.SystemActing(ctx, contactShareActor), tx, approvals.StageInput{
		Kind:           contactShareKind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     string(recordTypeContact),
		TargetID:       req.ContactID.UUID,
		Summary:        summary,
		JoinPending:    true,
		Identity:       identityKey,
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		// The owner left after the contact row was read.
		return false, nil
	}
	return open, err
}

// contactShareSummary is the card's sentence, naming the colleague who asked.
func contactShareSummary(ctx context.Context, tx pgx.Tx, requester ids.UUID) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM app_user WHERE id = $1`, requester).Scan(&name); err != nil {
		return "", fmt.Errorf("compose: naming the colleague who asked to share a contact: %w", err)
	}
	return fmt.Sprintf(approvalSummaryCopyIn(ctx, tx).contactShareAsked, name), nil
}

// contactShareAcceptEffect shares the contact with the colleague who asked:
// Write, the access Share gives, or Read for a seat that may not write. The
// contact's own visibility does not change.
//
// Decline needs no effect: the request closes, and StageUnlessDeclined files
// no second one.
func contactShareAcceptEffect(svc *approvals.Service, grants *identity.Service) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var req approvalsubject.ContactShare
		if err := json.Unmarshal(proposedChange, &req); err != nil {
			return fmt.Errorf("compose: decoding the share request: %w", err)
		}
		// Executed as the deciding owner, whose grants the share is checked
		// against as if they pressed Share.
		return svc.RedeemAndApply(ctx, approvalID, contactShareKind, diffHash, func(tx pgx.Tx) error {
			access, err := identity.ShareAccessForTx(ctx, tx, req.RequesterID)
			if err != nil {
				return err
			}
			return grants.CreateRecordGrantTx(ctx, tx, identity.CreateGrantInput{
				RecordType:  string(recordTypeContact),
				RecordID:    req.ContactID,
				SubjectType: entityUser,
				SubjectID:   req.RequesterID,
				Access:      access,
			})
		})
	}
}
