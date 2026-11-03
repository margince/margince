// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who may still give a record and who may still take it. The offer, the
// candidate list, the decision precheck and the locked move all ask these
// functions. A colleague the picker offers is then one the move accepts.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var (
	// errOfferGiverGone refuses an accept once the colleague who offered the
	// record is suspended or departed, or may no longer change it.
	errOfferGiverGone = &contacts.TransferRefusal{
		Code:    "owner_unavailable",
		Message: "The colleague who offered this record can no longer hand it over. Decline the offer, or ask an administrator.",
	}
	// errReceiverUnavailable refuses an accept once the colleague receiving the
	// record can no longer hold it.
	errReceiverUnavailable = &contacts.TransferRefusal{
		Code:    "receiver_unavailable",
		Message: "The colleague receiving this record can no longer take it on: they must be an active colleague who may change records of this kind.",
	}
)

// handOverRecipientsTx answers which candidates may receive a record of
// recordType. Each is a live human seat that can write, as
// EnsureHandOverRecipient rules, and their role may read and change the type.
func handOverRecipientsTx(ctx context.Context, tx pgx.Tx, recordType string, candidates []ids.UUID) (map[ids.UUID]bool, error) {
	seated, err := auth.EligibleHandOverRecipients(ctx, tx, candidates)
	if err != nil {
		return nil, err
	}
	var asked []ids.UUID
	for _, candidate := range candidates {
		if seated[candidate] {
			asked = append(asked, candidate)
		}
	}
	members, err := identity.MemberPrincipalsTx(ctx, tx, asked)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(members))
	for id, member := range members {
		out[id] = member.Permissions.Allows(recordType, principal.ActionRead) &&
			member.Permissions.Allows(recordType, principal.ActionUpdate)
	}
	return out, nil
}

// receiverEligibleTx holds the receiver's seat to commit and answers whether
// they may still receive a record of recordType.
func receiverEligibleTx(ctx context.Context, tx pgx.Tx, recordType string, to ids.UUID) (bool, error) {
	if err := auth.EnsureHandOverRecipient(ctx, tx, to); err != nil {
		if _, ineligible := errors.AsType[*auth.AssigneeNotAllowedError](err); ineligible {
			return false, nil
		}
		return false, err
	}
	eligible, err := handOverRecipientsTx(ctx, tx, recordType, []ids.UUID{to})
	return eligible[to], err
}

// transferPartiesTx answers the context the move runs in, after re-reading
// both parties on tx. The giver of a request decided it, so the caller is the
// giver. The giver of an offer is read again by offeredGiverTx.
func transferPartiesTx(ctx context.Context, tx pgx.Tx, transfer approvalsubject.OwnershipTransfer) (context.Context, error) {
	giverCtx := ctx
	if transfer.Offered() {
		var err error
		if giverCtx, err = offeredGiverTx(ctx, tx, transfer); err != nil {
			return nil, err
		}
	} else if err := auth.EnsureWritableLive(ctx, tx, transfer.RecordType, transfer.RecordID); err != nil {
		return nil, err
	}
	eligible, err := receiverEligibleTx(giverCtx, tx, transfer.RecordType, transfer.ToID)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, errReceiverUnavailable
	}
	return giverCtx, nil
}

// offeredGiverTx binds the giver of an offer from their stored seat and
// grants, and refuses unless they may still change the record.
func offeredGiverTx(ctx context.Context, tx pgx.Tx, transfer approvalsubject.OwnershipTransfer) (context.Context, error) {
	givers, err := identity.MemberPrincipalsTx(ctx, tx, []ids.UUID{transfer.FromID})
	if err != nil {
		return nil, err
	}
	giver, ok := givers[transfer.FromID]
	if !ok {
		return nil, errOfferGiverGone
	}
	giverCtx := principal.WithActor(ctx, giver)
	writable, err := auth.WritableLiveBy(giverCtx, tx, transfer.RecordType, transfer.RecordID)
	if err != nil {
		return nil, err
	}
	if !writable {
		return nil, errOfferGiverGone
	}
	return giverCtx, nil
}

// offerCandidate is a colleague the owner may offer the record to.
type offerCandidate struct {
	ID          ids.UUID
	DisplayName string
}

// OfferCandidates names the colleagues the caller may offer their own record
// to, by the rule the offer and its accept apply.
func (t ownershipTransfers) OfferCandidates(ctx context.Context, recordType string, id ids.UUID) ([]offerCandidate, error) {
	var out []offerCandidate
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		giver, err := requireOfferer(ctx, recordType, ids.Nil)
		if err != nil {
			return err
		}
		if err := ownRecordTx(ctx, tx, recordType, id, giver); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, display_name FROM app_user
			 WHERE id <> $1 AND NOT is_agent ORDER BY display_name, id`, giver)
		if err != nil {
			return fmt.Errorf("compose: reading colleagues to offer a record to: %w", err)
		}
		colleagues, err := pgx.CollectRows(rows, pgx.RowToStructByPos[offerCandidate])
		if err != nil {
			return fmt.Errorf("compose: reading colleagues to offer a record to: %w", err)
		}
		candidates := make([]ids.UUID, len(colleagues))
		for i, colleague := range colleagues {
			candidates[i] = colleague.ID
		}
		eligible, err := handOverRecipientsTx(ctx, tx, recordType, candidates)
		if err != nil {
			return err
		}
		for _, colleague := range colleagues {
			if eligible[colleague.ID] {
				out = append(out, colleague)
			}
		}
		return nil
	})
	return out, err
}
