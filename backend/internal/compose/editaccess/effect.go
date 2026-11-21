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
var errOwnerMoved = &contacts.OwnerAskRefusal{
	Code:    "owner_changed",
	Message: "This record has changed owner since the request was made, so its access cannot be given from here. Decline the request.",
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
			if err := grants.ShareWithRequesterTx(ctx, tx, ask.RecordType, ask.RecordID, ask.RequesterID); err != nil {
				return err
			}
			return tellOutcome(ctx, tx, approvalID, ask, accepted)
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

// FollowOwnerTx closes every open request on the record that was put to an
// owner other than to, on the transaction that moved it. The owner's line about
// each is taken back, and the colleague who asked is told. to is nil for a
// record nobody owns.
func FollowOwnerTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID, to *ids.UUID) error {
	gone, err := approvals.FollowEditAccessOwnerInTx(ctx, tx, recordType, id, to)
	if err != nil {
		return err
	}
	for _, request := range gone {
		if _, err := notices.NewStore(nil).Retract(ctx, tx, notices.ApprovalNoticeKey(request.ID.UUID)); err != nil {
			return err
		}
		ask, err := decode(request.ProposedChange)
		if err != nil {
			return err
		}
		if err := tellOutcome(ctx, tx, request.ID, ask, closed); err != nil {
			return err
		}
	}
	return nil
}

// outcome is how a request ended, told to the colleague who asked.
type outcome struct{ kind, subject, body string }

var (
	accepted = outcome{"edit_access_accepted", "editAccess.Accepted", "editAccess.AcceptedBody"}
	declined = outcome{"edit_access_declined", "editAccess.Declined", "editAccess.DeclinedBody"}
	expired  = outcome{"edit_access_expired", "editAccess.Expired", "editAccess.ExpiredBody"}
	closed   = outcome{"edit_access_withdrawn", "editAccess.Withdrawn", "editAccess.WithdrawnBody"}
)

// tellOutcome gives the colleague who asked one line naming the owner, once per
// request and outcome. It names the record only while they may still read it.
func tellOutcome(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, ask approvalsubject.EditAccess, end outcome) error {
	owner, err := recordowner.ColleagueNameTx(ctx, tx, ask.OwnerID)
	if err != nil {
		return err
	}
	lang := identity.BaseLanguageForRecord(ctx, tx)
	record, err := recordowner.NameForTx(ctx, tx, ask.RequesterID, ask.RecordType, ask.RecordID, lang)
	if err != nil {
		return err
	}
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
		Subject: fmt.Sprintf(systemtext.Copy(end.subject, lang), owner, record),
		Body:    systemtext.Copy(end.body, lang),
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
