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
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	// Kind is the approval kind an edit access request is filed under.
	Kind = approvalsubject.KindEditAccess
	// window keeps a request open across a week's absence, as a handover's.
	window    = 7 * 24 * time.Hour
	withdrawn = "the colleague who asked withdrew it"
)

var errOwnerUnavailable = &contacts.TransferRefusal{
	Code:    "owner_unavailable",
	Message: "The owner of this record cannot be asked, because they are suspended or have left. Ask an administrator for edit access.",
}

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
		if out, err = ownTx(ctx, tx, recordType, id); !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if err := e.stageTx(ctx, tx, recordType, id, record.OwnerID); err != nil {
			return err
		}
		created = true
		out, err = ownTx(ctx, tx, recordType, id)
		return err
	})
	return out, created, err
}

// stageTx files the caller's request in the owner's inbox. A second press by
// the same colleague joins the open one.
func (e Engine) stageTx(ctx context.Context, tx pgx.Tx, recordType string, id, owner ids.UUID) error {
	asker, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("editaccess: a request without a caller")
	}
	askable, err := approvals.CanBeAsked(ctx, tx, owner)
	if err != nil {
		return err
	}
	if !askable {
		return errOwnerUnavailable
	}
	body, err := json.Marshal(approvalsubject.EditAccess{
		RecordType: recordType, RecordID: id, OwnerID: owner, RequesterID: asker.UserID,
	})
	if err != nil {
		return fmt.Errorf("editaccess: encoding the request: %w", err)
	}
	identityKey, err := json.Marshal(map[string]ids.UUID{"requester_id": asker.UserID})
	if err != nil {
		return fmt.Errorf("editaccess: encoding the request's identity: %w", err)
	}
	name, err := colleagueName(ctx, tx, asker.UserID)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	ttl := window
	_, err = e.staging.StageOrJoinPendingInTx(ctx, tx, approvals.StageInput{
		Kind:           Kind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     recordType,
		TargetID:       id,
		Summary:        fmt.Sprintf(phrase("editAccess.Asked", identity.BaseLanguageForRecord(ctx, tx)), name),
		SystemText:     &systemtext.Message{Version: 1, Subject: systemtext.Sentence("editAccess.Asked", systemtext.Label(name))},
		JoinPending:    true,
		Identity:       identityKey,
		TTL:            &ttl,
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return errOwnerUnavailable
	}
	return err
}

// Own answers the caller's open request on the record.
func (e Engine) Own(ctx context.Context, recordType string, id ids.UUID) (Request, error) {
	var out Request
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = ownTx(ctx, tx, recordType, id)
		return err
	})
	return out, err
}

// Withdraw takes the caller's open request back out of the owner's inbox.
func (e Engine) Withdraw(ctx context.Context, recordType string, id ids.UUID) error {
	return e.db.Tx(ctx, func(tx pgx.Tx) error {
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

// colleagueName is a member's display name, for a sentence naming them.
func colleagueName(ctx context.Context, tx pgx.Tx, member ids.UUID) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM app_user WHERE id = $1`, member).Scan(&name); err != nil {
		return "", fmt.Errorf("editaccess: naming a colleague: %w", err)
	}
	return name, nil
}

// phrase is one sentence of this request's copy in lang.
func phrase(code string, lang textlang.Lang) string { return systemtext.Copy(code, lang) }
