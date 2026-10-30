// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Moving a contact or company between two colleagues who both agree. One asks,
// the other answers in their approvals inbox. contacts says who may ask,
// approvals holds the request, and contacts, deals and activities each write
// their own rows when it is accepted. No module imports another, so they meet
// here.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	ownershipTransferKind = approvalsubject.KindOwnershipTransfer
	// ownershipTransferWindow keeps a request open across a week's absence.
	ownershipTransferWindow = 7 * 24 * time.Hour
	ownershipNoteMaxRunes   = 500
	ownershipWithdrawn      = "the colleague who asked withdrew the request"
)

// ownershipTransfers is the one engine behind the request routes and the
// decision effects.
type ownershipTransfers struct {
	db *database.DB
	// staging files requests; the decisions run on the service the effects
	// below are registered with.
	staging  *approvals.Service
	contacts *contacts.Store
	deals    *deals.Store
	tasks    *activities.Store
}

func newOwnershipTransfers(pool *pgxpool.Pool) ownershipTransfers {
	db := InstallationDB(pool)
	return ownershipTransfers{
		db:       db,
		staging:  NewApprovalsService(db),
		contacts: newContactsStore(db).WithShareRequests(contactShareRequests(pool)),
		deals:    deals.NewStore(db, DealsInstallation()),
		tasks:    activities.NewStore(db),
	}
}

// ownershipRequestView is a request as its asker reads it.
type ownershipRequestView struct {
	ApprovalID ids.ApprovalID
	Transfer   approvalsubject.OwnershipTransfer
	ExpiresAt  time.Time
}

// ownershipNoteError refuses a note longer than the card holds.
type ownershipNoteError struct{}

func (e *ownershipNoteError) Error() string {
	return fmt.Sprintf("the note is longer than %d characters", ownershipNoteMaxRunes)
}

// FieldFault names the note.
func (e *ownershipNoteError) FieldFault() (field, code, message string) {
	return bulkParamNote, "too_long", e.Error()
}

var errOwnerUnavailable = &contacts.TransferRefusal{
	Code:    "owner_unavailable",
	Message: "The owner of this record cannot be asked, because they are suspended or have left. Ask an administrator to hand it over.",
}

// Request asks the record's owner to hand it to the caller. created is false
// when the caller's open request already stood and is answered instead.
func (t ownershipTransfers) Request(ctx context.Context, recordType string, id ids.UUID, note string) (ownershipRequestView, bool, error) {
	if utf8.RuneCountInString(note) > ownershipNoteMaxRunes {
		return ownershipRequestView{}, false, &ownershipNoteError{}
	}
	var out ownershipRequestView
	created := false
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		record, err := contacts.TransferRequestableTx(ctx, tx, recordType, id)
		if err != nil {
			return err
		}
		if out, err = ownRequestTx(ctx, tx, recordType, id); err == nil {
			return nil
		} else if !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if err := t.stageTx(ctx, tx, recordType, id, record.OwnerID, note); err != nil {
			return err
		}
		created = true
		out, err = ownRequestTx(ctx, tx, recordType, id)
		return err
	})
	return out, created, err
}

// stageTx files the request in the owner's inbox.
func (t ownershipTransfers) stageTx(ctx context.Context, tx pgx.Tx, recordType string, id, owner ids.UUID, note string) error {
	asker, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("compose: a handover request without a caller")
	}
	askable, err := approvals.CanBeAsked(ctx, tx, owner)
	if err != nil {
		return err
	}
	if !askable {
		return errOwnerUnavailable
	}
	transfer := approvalsubject.OwnershipTransfer{
		RecordType: recordType, RecordID: id, FromID: owner, ToID: asker.UserID,
		RequestedBy: asker.UserID, DeciderID: owner, Note: note,
	}
	body, err := json.Marshal(transfer)
	if err != nil {
		return fmt.Errorf("compose: encoding the handover request: %w", err)
	}
	identityKey, err := json.Marshal(map[string]ids.UUID{"requested_by": asker.UserID, "to_id": asker.UserID})
	if err != nil {
		return fmt.Errorf("compose: encoding the handover request's identity: %w", err)
	}
	summary, err := t.askedSummary(ctx, tx, transfer)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	window := ownershipTransferWindow
	_, err = t.staging.StageOrJoinPendingInTx(ctx, tx, approvals.StageInput{
		Kind:           ownershipTransferKind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     recordType,
		TargetID:       id,
		Summary:        summary,
		JoinPending:    true,
		Identity:       identityKey,
		TTL:            &window,
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return errOwnerUnavailable
	}
	return err
}

// askedSummary is the card's sentence: who asks, and what moves with the
// record. The counts are the giver's, read when the request is filed through
// the selectors the move itself uses.
func (t ownershipTransfers) askedSummary(ctx context.Context, tx pgx.Tx, transfer approvalsubject.OwnershipTransfer) (string, error) {
	asker, err := colleagueName(ctx, tx, transfer.RequestedBy)
	if err != nil {
		return "", err
	}
	from := ids.From[ids.UserKind](transfer.FromID)
	dealCount, err := countOrNone(t.deals.CountRecordDealsHeldByTx(ctx, tx, transfer.RecordType, transfer.RecordID, from))
	if err != nil {
		return "", err
	}
	link := activities.RecordLink{EntityType: transfer.RecordType, ID: transfer.RecordID}
	taskCount, err := countOrNone(t.tasks.CountRecordTasksHeldByTx(ctx, tx, link, from))
	if err != nil {
		return "", err
	}
	phrase := ownershipTransferWords.AskedContact
	if transfer.RecordType == string(recordTypeCompany) {
		phrase = ownershipTransferWords.AskedCompany
	}
	return fmt.Sprintf(phrase.In(identity.BaseLanguageForRecord(ctx, tx)), asker, dealCount.Moved, taskCount.Moved), nil
}

// countOrNone reads a refused count as nothing to count.
func countOrNone[T any](n T, err error) (T, error) {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		var none T
		return none, nil
	}
	return n, err
}

// Own answers the caller's open request on the record.
func (t ownershipTransfers) Own(ctx context.Context, recordType string, id ids.UUID) (ownershipRequestView, error) {
	var out ownershipRequestView
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = ownRequestTx(ctx, tx, recordType, id)
		return err
	})
	return out, err
}

// Withdraw takes the caller's open request back out of the owner's inbox.
func (t ownershipTransfers) Withdraw(ctx context.Context, recordType string, id ids.UUID) error {
	return t.db.Tx(ctx, func(tx pgx.Tx) error {
		withdrawn, err := approvals.WithdrawOwnRequestTx(ctx, tx, ownershipTransferKind, recordType, id, ownershipWithdrawn)
		if err != nil {
			return err
		}
		return retractApprovalNotice(ctx, tx, withdrawn)
	})
}

func ownRequestTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (ownershipRequestView, error) {
	own, err := approvals.OwnRequestForTargetTx(ctx, tx, ownershipTransferKind, recordType, id)
	if err != nil {
		return ownershipRequestView{}, err
	}
	out := ownershipRequestView{ApprovalID: own.ID, ExpiresAt: own.ExpiresAt}
	if err := json.Unmarshal(own.ProposedChange, &out.Transfer); err != nil {
		return ownershipRequestView{}, fmt.Errorf("compose: decoding a handover request: %w", err)
	}
	return out, nil
}

// colleagueName is a member's display name, for a sentence naming them.
func colleagueName(ctx context.Context, tx pgx.Tx, member ids.UUID) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM app_user WHERE id = $1`, member).Scan(&name); err != nil {
		return "", fmt.Errorf("compose: naming a colleague in a handover: %w", err)
	}
	return name, nil
}
