// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package editaccess

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/recordowner"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// errOwnerMoved refuses an approval once the record has left the owner the
// request was put to.
var errOwnerMoved = &contacts.TransferRefusal{
	Code:    "owner_changed",
	Message: "This record has changed owner since the request was made, so its access cannot be given from here. Decline the request.",
}

// ShareWithRequesterTx shares a record with the colleague who asked for it:
// write, or read for a seat that may not write. The share is checked against
// the caller's grants as if they pressed Share.
func ShareWithRequesterTx(ctx context.Context, tx pgx.Tx, grants *identity.Service, recordType string, recordID, requester ids.UUID) error {
	access, err := identity.ShareAccessForTx(ctx, tx, requester)
	if err != nil {
		return err
	}
	return grants.CreateRecordGrantTx(ctx, tx, identity.CreateGrantInput{
		RecordType:  recordType,
		RecordID:    recordID,
		SubjectType: "user",
		SubjectID:   requester,
		Access:      access,
	})
}

// acceptEffect shares the record with the colleague who asked, as the owner
// deciding it. The owner is read again under the decision first.
func acceptEffect(svc *approvals.Service, grants *identity.Service) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		ask, err := decode(proposedChange)
		if err != nil {
			return err
		}
		decider, ok := principal.Actor(ctx)
		if !ok || decider.UserID.IsZero() || decider.UserID != ask.OwnerID {
			return fmt.Errorf("edit access is given by the owner it is put to: %w", apperrors.ErrPermissionDenied)
		}
		return svc.RedeemAndApply(ctx, approvalID, Kind, diffHash, func(tx pgx.Tx) error {
			if err := ownerUnmovedTx(ctx, tx, ask); err != nil {
				return err
			}
			if err := ShareWithRequesterTx(ctx, tx, grants, ask.RecordType, ask.RecordID, ask.RequesterID); err != nil {
				return err
			}
			return tellOutcome(ctx, tx, approvalID, proposedChange, accepted)
		})
	}
}

// precheck refuses an approval before it is decided, so the card stays pending
// and can still be declined.
func precheck(db *database.DB) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, _ json.RawMessage) error {
		ask, err := decode(staged)
		if err != nil {
			return err
		}
		return db.Tx(ctx, func(tx pgx.Tx) error { return ownerUnmovedTx(ctx, tx, ask) })
	}
}

func ownerUnmovedTx(ctx context.Context, tx pgx.Tx, ask approvalsubject.EditAccess) error {
	owner, _, err := recordowner.ReadTx(ctx, tx, ask.RecordType, ask.RecordID)
	if err != nil {
		return err
	}
	if owner == nil || *owner != ask.OwnerID {
		return errOwnerMoved
	}
	return nil
}

// outcome is how a request ended, told to the colleague who asked.
type outcome struct{ kind, subject, body string }

var (
	accepted = outcome{"edit_access_accepted", "editAccess.Accepted", "editAccess.AcceptedBody"}
	declined = outcome{"edit_access_declined", "editAccess.Declined", "editAccess.DeclinedBody"}
	expired  = outcome{"edit_access_expired", "editAccess.Expired", "editAccess.ExpiredBody"}
)

// tellOutcome gives the colleague who asked one line naming the owner and the
// record, once per request and outcome.
func tellOutcome(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, proposedChange json.RawMessage, end outcome) error {
	ask, err := decode(proposedChange)
	if err != nil {
		return err
	}
	owner, err := colleagueName(ctx, tx, ask.OwnerID)
	if err != nil {
		return err
	}
	_, record, err := recordowner.ReadTx(ctx, tx, ask.RecordType, ask.RecordID)
	if err != nil {
		return err
	}
	lang := identity.BaseLanguageForRecord(ctx, tx)
	_, err = notices.NewStore(nil).CreateTx(ctx, tx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](ask.RequesterID),
		Kind:      end.kind,
		Target:    notices.Target{Type: ask.RecordType, ID: ask.RecordID},
		DedupeKey: fmt.Sprintf("%s:%s:%s", Kind, approvalID, end.kind),
		SystemText: &systemtext.Message{
			Version: 1,
			Subject: systemtext.Sentence(end.subject, systemtext.Label(owner), systemtext.Label(record)),
			Body:    systemtext.Sentence(end.body),
		},
		Subject: fmt.Sprintf(phrase(end.subject, lang), owner, record),
		Body:    phrase(end.body, lang),
	})
	return err
}

func decode(raw json.RawMessage) (approvalsubject.EditAccess, error) {
	var ask approvalsubject.EditAccess
	if err := json.Unmarshal(raw, &ask); err != nil {
		return approvalsubject.EditAccess{}, fmt.Errorf("editaccess: decoding a request: %w", err)
	}
	return ask, nil
}
