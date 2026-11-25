// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Moving a contact or company between two colleagues who both agree. One asks
// for it or offers it, the other answers in their approvals inbox. contacts says who may ask,
// approvals holds the request, and contacts, deals and activities each write
// their own rows when it is accepted. No module imports another, so they meet
// here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/recordowner"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const (
	ownershipTransferKind = approvalsubject.KindOwnershipTransfer
	// ownershipTransferWindow keeps a request open across a week's absence.
	ownershipTransferWindow = 7 * 24 * time.Hour
	ownershipNoteMaxRunes   = 500
	ownershipWithdrawn      = "the colleague who filed it withdrew it"
)

// ownershipTransfers is the one engine behind the request and offer routes and
// the decision effects.
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

// FieldMax is the note's limit.
func (e *ownershipNoteError) FieldMax() int { return ownershipNoteMaxRunes }

var errOwnerUnavailable = &contacts.OwnerAskRefusal{
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
		if !transferRecords[recordType] {
			return &contacts.InvalidRecordTypeError{RecordType: recordType}
		}
		record, err := contacts.OwnerAskableTx(ctx, tx, recordType, id, func() (*ids.UUID, string, error) {
			return recordowner.ReadTx(ctx, tx, recordType, id)
		})
		if err != nil {
			return err
		}
		if out, err = ownRequestTx(ctx, tx, recordType, id, transferAsked); err == nil {
			return nil
		} else if !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if err := refuseBesideOffer(ctx, tx, recordType, id); err != nil {
			return err
		}
		if err := t.stageRequestTx(ctx, tx, recordType, id, record.OwnerID, note); err != nil {
			return err
		}
		created = true
		out, err = ownRequestTx(ctx, tx, recordType, id, transferAsked)
		return err
	})
	return out, created, err
}

// stageRequestTx files the caller's request in the owner's inbox.
func (t ownershipTransfers) stageRequestTx(ctx context.Context, tx pgx.Tx, recordType string, id, owner ids.UUID, note string) error {
	asker, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("compose: a handover request without a caller")
	}
	err := recordowner.AskableTx(ctx, tx, owner)
	if err == nil {
		err = t.fileTx(ctx, tx, approvalsubject.OwnershipTransfer{
			RecordType: recordType, RecordID: id, FromID: owner, ToID: asker.UserID,
			RequestedBy: asker.UserID, DeciderID: owner, Note: note,
		})
	}
	if errors.Is(err, recordowner.ErrOwnerCannotBeAsked) {
		return errOwnerUnavailable
	}
	return err
}

// fileTx files a transfer in its decider's inbox. A second press by the same
// colleague for the same receiver joins the open one.
func (t ownershipTransfers) fileTx(ctx context.Context, tx pgx.Tx, transfer approvalsubject.OwnershipTransfer) error {
	summary, metadata, err := t.askedSummary(ctx, tx, transfer)
	if err != nil {
		return err
	}
	_, err = recordowner.FileTx(ctx, tx, t.staging, recordowner.Request{
		Kind: ownershipTransferKind, RecordType: transfer.RecordType, RecordID: transfer.RecordID,
		Payload: transfer, Identity: map[string]ids.UUID{"requested_by": transfer.RequestedBy, "to_id": transfer.ToID},
		Summary: summary, SystemText: metadata, TTL: ownershipTransferWindow,
	})
	return err
}

// askedSummary is the card's sentence: who staged it, and what moves with the
// record. The counts are the giver's, read when the transfer is filed through
// the selectors the move itself uses.
func (t ownershipTransfers) askedSummary(ctx context.Context, tx pgx.Tx, transfer approvalsubject.OwnershipTransfer) (string, *systemtext.Message, error) {
	asker, err := recordowner.ColleagueNameTx(ctx, tx, transfer.RequestedBy)
	if err != nil {
		return "", nil, err
	}
	from := ids.From[ids.UserKind](transfer.FromID)
	dealCount, err := countOrNone(t.deals.CountRecordDealsHeldByTx(ctx, tx, transfer.RecordType, transfer.RecordID, from))
	if err != nil {
		return "", nil, err
	}
	link := activities.RecordLink{EntityType: transfer.RecordType, ID: transfer.RecordID}
	taskCount, err := countOrNone(t.tasks.CountRecordTasksHeldByTx(ctx, tx, link, from))
	if err != nil {
		return "", nil, err
	}
	phrase := cardPhrase(transfer)
	return fmt.Sprintf(phrase.In(identity.BaseLanguageForRecord(ctx, tx)), asker, dealCount.Moved, taskCount.Moved), &systemtext.Message{Subject: systemtext.Sentence(cardCode(transfer), systemtext.Label(asker), systemtext.Count(int64(dealCount.Moved)), systemtext.Count(int64(taskCount.Moved)))}, nil
}

// countOrNone reads a refused count as nothing to count.
func countOrNone[T any](n T, err error) (T, error) {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		var none T
		return none, nil
	}
	return n, err
}

// transferDirection is which party staged a transfer: the receiver asking, or
// the giver offering.
type transferDirection bool

const (
	transferAsked   transferDirection = false
	transferOffered transferDirection = true
)

// Own answers the caller's open transfer of one direction on the record.
func (t ownershipTransfers) Own(ctx context.Context, recordType string, id ids.UUID, direction transferDirection) (ownershipRequestView, error) {
	var out ownershipRequestView
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = ownRequestTx(ctx, tx, recordType, id, direction)
		return err
	})
	return out, err
}

// Withdraw takes the caller's open transfer of one direction back out of the
// other party's inbox.
func (t ownershipTransfers) Withdraw(ctx context.Context, recordType string, id ids.UUID, direction transferDirection) error {
	return t.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := ownRequestTx(ctx, tx, recordType, id, direction); err != nil {
			return err
		}
		withdrawn, err := approvals.WithdrawOwnRequestTx(ctx, tx, ownershipTransferKind, recordType, id, ownershipWithdrawn)
		if err != nil {
			return err
		}
		return retractApprovalNotice(ctx, tx, withdrawn)
	})
}

// ownRequestTx answers the caller's open transfer on the record when it runs
// in direction. A record the caller may not see is ErrNotFound; a visible one
// with no such transfer is ErrNothingYet.
func ownRequestTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID, direction transferDirection) (ownershipRequestView, error) {
	if err := auth.EnsureVisibleLive(ctx, tx, recordType, id); err != nil {
		return ownershipRequestView{}, err
	}
	own, err := approvals.OwnRequestForTargetTx(ctx, tx, ownershipTransferKind, recordType, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return ownershipRequestView{}, apperrors.ErrNothingYet
	}
	if err != nil {
		return ownershipRequestView{}, err
	}
	out := ownershipRequestView{ApprovalID: own.ID, ExpiresAt: own.ExpiresAt}
	if err := json.Unmarshal(own.ProposedChange, &out.Transfer); err != nil {
		return ownershipRequestView{}, fmt.Errorf("compose: decoding a handover request: %w", err)
	}
	if transferDirection(out.Transfer.Offered()) != direction {
		return ownershipRequestView{}, apperrors.ErrNothingYet
	}
	return out, nil
}
