// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package editaccess is a colleague's request for edit access to a record they
// can see but not change. The owner answers it in their inbox. Approving gives
// the colleague a write share, and the owner stays the owner.
//
// contacts says who may ask, approvals holds the request, and identity writes
// the share. No module imports another, so they meet here.
package editaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/recordowner"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const (
	// Kind is the approval kind an edit access request is filed under.
	Kind = approvalsubject.KindEditAccess
	// window keeps a request open across a week's absence, as a handover's.
	window    = 7 * 24 * time.Hour
	withdrawn = "the colleague who asked withdrew it"
)

var (
	errOwnerUnavailable = &contacts.OwnerAskRefusal{
		Code:    "owner_unavailable",
		Message: "The owner of this record cannot be asked, because they are suspended or have left. Ask an administrator for edit access.",
	}
	errDeclined = &contacts.OwnerAskRefusal{
		Code:    "declined",
		Message: "The owner declined your request for edit access to this record, so it is not sent again. Talk to them if something changed.",
	}
)

// Engine files, reads and withdraws a colleague's own request.
type Engine struct {
	db *database.DB
	// staging files requests; the decisions run on the service Register is
	// called with.
	staging *approvals.Service
}

// New builds the engine over the installation's database.
func New(db *database.DB, staging *approvals.Service) Engine {
	return Engine{db: db, staging: staging}
}

// Request is a request as the colleague who asked reads it.
type Request struct {
	ApprovalID ids.ApprovalID
	Ask        approvalsubject.EditAccess
	ExpiresAt  time.Time
}

// Ask puts the request in the owner's inbox. created is false when the
// caller's open request already stood and is answered instead.
func (e Engine) Ask(ctx context.Context, recordType string, id ids.UUID) (Request, bool, error) {
	if !recordowner.Known(recordType) {
		return Request{}, false, &contacts.InvalidRecordTypeError{RecordType: recordType}
	}
	var out Request
	created := false
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		record, err := contacts.OwnerAskableTx(ctx, tx, recordType, id, func() (*ids.UUID, string, error) {
			return recordowner.ReadTx(ctx, tx, recordType, id)
		})
		if err != nil {
			return err
		}
		// A lead and a project tell nobody when they change owner. So a
		// request put to a former owner is closed here, before it is answered.
		if err := FollowOwnerTx(ctx, tx, recordType, id, &record.OwnerID); err != nil {
			return err
		}
		if out, err = ownTx(ctx, tx, recordType, id); !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if err := e.fileTx(ctx, tx, recordType, id, record.OwnerID); err != nil {
			return err
		}
		created = true
		out, err = ownTx(ctx, tx, recordType, id)
		return err
	})
	return out, created, err
}

// fileTx files the caller's request in the owner's inbox, unless this owner
// already declined it.
func (e Engine) fileTx(ctx context.Context, tx pgx.Tx, recordType string, id, owner ids.UUID) error {
	asker, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("editaccess: a request without a caller")
	}
	if err := recordowner.AskableTx(ctx, tx, owner); err != nil {
		return ownerRefusal(err)
	}
	name, err := recordowner.ColleagueNameTx(ctx, tx, asker.UserID)
	if err != nil {
		return err
	}
	filed, err := recordowner.FileTx(ctx, tx, e.staging, recordowner.Request{
		Kind: Kind, RecordType: recordType, RecordID: id,
		Payload: approvalsubject.EditAccess{RecordType: recordType, RecordID: id, OwnerID: owner, RequesterID: asker.UserID},
		// The owner is part of the request, so a new owner may be asked after
		// the last one declined.
		Identity:     map[string]ids.UUID{"requester_id": asker.UserID, "owner_id": owner},
		Summary:      fmt.Sprintf(systemtext.Copy("editAccess.Asked", identity.BaseLanguageForRecord(ctx, tx)), name),
		SystemText:   &systemtext.Message{Version: 1, Subject: systemtext.Sentence("editAccess.Asked", systemtext.Label(name))},
		TTL:          window,
		OnceDeclined: true,
	})
	if err != nil {
		return ownerRefusal(err)
	}
	if !filed {
		return errDeclined
	}
	return nil
}

func ownerRefusal(err error) error {
	if errors.Is(err, recordowner.ErrOwnerCannotBeAsked) {
		return errOwnerUnavailable
	}
	return err
}

// Own answers the caller's open request on a record they may still read.
func (e Engine) Own(ctx context.Context, recordType string, id ids.UUID) (Request, error) {
	var out Request
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, recordType, id); err != nil {
			return err
		}
		var err error
		out, err = ownTx(ctx, tx, recordType, id)
		return err
	})
	return out, err
}

// Withdraw takes the caller's open request back out of the owner's inbox. An
// archived record still takes it back: withdrawing writes nothing on it.
func (e Engine) Withdraw(ctx context.Context, recordType string, id ids.UUID) error {
	return e.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, recordType, id); err != nil {
			return err
		}
		gone, err := approvals.WithdrawOwnRequestTx(ctx, tx, Kind, recordType, id, withdrawn)
		if err != nil {
			return err
		}
		_, err = notices.NewStore(nil).Retract(ctx, tx, notices.ApprovalNoticeKey(gone.UUID))
		return err
	})
}

// ownTx answers the caller's open request on the record, else
// apperrors.ErrNotFound.
func ownTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (Request, error) {
	own, err := approvals.OwnRequestForTargetTx(ctx, tx, Kind, recordType, id)
	if err != nil {
		return Request{}, err
	}
	out := Request{ApprovalID: own.ID, ExpiresAt: own.ExpiresAt}
	if err := json.Unmarshal(own.ProposedChange, &out.Ask); err != nil {
		return Request{}, fmt.Errorf("editaccess: decoding a request: %w", err)
	}
	return out, nil
}
