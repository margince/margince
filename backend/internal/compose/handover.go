// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The handover of a member's work. Every live record they own moves to a named
// colleague through that record's own owner-change write. So each record gets
// the audit row, event and follow-on moves one reassignment writes, a deal's
// pending proposals included. Deactivating a member moves nothing.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// handoverRecordKinds are the record types the bulk reassignment moves, in the
// order a handover takes them.
var handoverRecordKinds = []crmcontracts.BulkRecordType{
	crmcontracts.BulkRecordTypeLead,
	crmcontracts.BulkRecordTypeContact,
	crmcontracts.BulkRecordTypeCompany,
	crmcontracts.BulkRecordTypeDeal,
}

// handoverSameUserError refuses a handover to the member themselves.
type handoverSameUserError struct{}

func (e *handoverSameUserError) Error() string {
	return "to_user_id names the member whose work is being handed over"
}

// FieldFault names the receiving colleague as the field refused.
func (e *handoverSameUserError) FieldFault() (field, code, message string) {
	return "to_user_id", "same_owner", e.Error()
}

// handoverReceiverError is auth.AssigneeNotAllowedError on the field this
// request sends.
type handoverReceiverError struct{ auth.AssigneeNotAllowedError }

// FieldFault names the receiving colleague as the field refused.
func (e *handoverReceiverError) FieldFault() (field, code, message string) {
	_, code, message = e.AssigneeNotAllowedError.FieldFault()
	return "to_user_id", code, message
}

// handoverSeam reuses the bulk engine's record targets, so a handover writes
// each record as a bulk reassignment of it would.
type handoverSeam struct {
	db      *database.DB
	targets map[crmcontracts.BulkRecordType]bulkTarget
}

func newHandoverSeam(engine *bulkEngine) handoverSeam {
	return handoverSeam{db: engine.db, targets: engine.targets}
}

// admit is the gate the count and the move both pass, in their own transaction.
// It admits a human whom the member verbs would let act on this member. Every
// record moved after it is still gated by its own write check.
func admit(ctx context.Context, tx pgx.Tx, member ids.UUID) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	return identity.AdmitMemberHandover(ctx, tx, ids.From[ids.UserKind](member))
}

// Owned counts the live records member owns that the caller can see.
func (s handoverSeam) Owned(ctx context.Context, member ids.UUID) (crmcontracts.HandoverCounts, error) {
	var counts crmcontracts.HandoverCounts
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := admit(ctx, tx, member); err != nil {
			return err
		}
		var err error
		counts, err = countOwned(ctx, tx, member)
		return err
	})
	return counts, err
}

func countOwned(ctx context.Context, tx pgx.Tx, member ids.UUID) (crmcontracts.HandoverCounts, error) {
	var counts crmcontracts.HandoverCounts
	for table, count := range handoverTally(&counts) {
		owned, err := visibleOwnedIDs(ctx, tx, table, member)
		if err != nil {
			return crmcontracts.HandoverCounts{}, err
		}
		*count = len(owned)
	}
	if !privateUnseen(ctx, member) {
		return counts, nil
	}
	private, err := contacts.OwnerPrivateCountsTx(ctx, tx, ids.From[ids.UserKind](member))
	counts.Contact += private.Contacts
	counts.Company += private.Companies
	return counts, err
}

// privateUnseen reports whether member's owner-private rows are hidden from the
// caller: from everyone but the member, whose visible count already holds them.
func privateUnseen(ctx context.Context, member ids.UUID) bool {
	caller, ok := principal.Actor(ctx)
	return !ok || caller.UserID != member
}

// visibleOwnedIDs is auth.VisibleOwnedIDs where a record type the caller may
// not read at all owns nothing they can count or move.
func visibleOwnedIDs(ctx context.Context, tx pgx.Tx, table string, member ids.UUID) ([]ids.UUID, error) {
	owned, err := auth.VisibleOwnedIDs(ctx, tx, table, member)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	return owned, err
}

// handoverTally names each counted table and the field its count lands in.
func handoverTally(counts *crmcontracts.HandoverCounts) map[string]*int {
	return map[string]*int{
		tableLead: &counts.Lead, tableContact: &counts.Contact, tableCompany: &counts.Company,
		tableDeal: &counts.Deal, tableProject: &counts.Project,
	}
}

// HandOver moves every live record member owns that the caller may change to
// receiver, and answers what moved and what member still owns. One
// transaction: the handover lands whole or not at all.
func (s handoverSeam) HandOver(ctx context.Context, member, receiver ids.UUID) (crmcontracts.UserHandoverResult, error) {
	var out crmcontracts.UserHandoverResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := admit(ctx, tx, member); err != nil {
			return err
		}
		if member == receiver {
			return &handoverSameUserError{}
		}
		if err := auth.EnsureAssignee(ctx, tx, receiver); err != nil {
			if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
				return &handoverReceiverError{}
			}
			return err
		}
		var err error
		out, err = s.handOverInTx(ctx, tx, member, ids.From[ids.UserKind](receiver))
		return err
	})
	return out, err
}

func (s handoverSeam) handOverInTx(
	ctx context.Context, tx pgx.Tx, member ids.UUID, receiver ids.UserID,
) (crmcontracts.UserHandoverResult, error) {
	var moved crmcontracts.HandoverCounts
	tally := handoverTally(&moved)
	for _, kind := range handoverRecordKinds {
		n, err := s.handOverKind(ctx, tx, kind, member, receiver)
		if err != nil {
			return crmcontracts.UserHandoverResult{}, err
		}
		*tally[string(kind)] = n
	}
	if privateUnseen(ctx, member) {
		private, err := contacts.HandOverOwnerPrivateTx(ctx, tx, ids.From[ids.UserKind](member), receiver)
		if err != nil {
			return crmcontracts.UserHandoverResult{}, err
		}
		moved.Contact += private.Contacts
		moved.Company += private.Companies
	}
	projectsMoved, err := projects.TransferProjectOwnershipTx(ctx, tx, projects.TransferProjectOwnershipInput{
		FromOwnerID: ids.From[ids.UserKind](member), ToOwnerID: receiver,
	})
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return crmcontracts.UserHandoverResult{}, err
	}
	moved.Project = projectsMoved
	left, err := countOwned(ctx, tx, member)
	return crmcontracts.UserHandoverResult{Moved: moved, Left: left}, err
}

// handOverKind moves one record type, each row in its own savepoint. A row its
// own write refuses stays with the member. An error that is no refusal aborts
// the handover.
func (s handoverSeam) handOverKind(
	ctx context.Context, tx pgx.Tx, kind crmcontracts.BulkRecordType, member ids.UUID, receiver ids.UserID,
) (int, error) {
	target := s.targets[kind]
	reassigner, err := reassignerOf(target)
	if err != nil {
		return 0, err
	}
	owned, err := visibleOwnedIDs(ctx, tx, string(kind), member)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, id := range owned {
		ok, err := handOverRow(ctx, tx, target, reassigner, id, member, receiver)
		if err != nil {
			return 0, err
		}
		if ok {
			moved++
		}
	}
	return moved, nil
}

// handOverRow re-reads the row under its lock, so a row another writer
// re-owned since the scan keeps that writer's owner.
func handOverRow(
	ctx context.Context, tx pgx.Tx, target bulkTarget, reassigner bulkReassigner,
	id, member ids.UUID, receiver ids.UserID,
) (bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("open a savepoint for %s: %w", id, err)
	}
	row, err := target.lock(ctx, savepoint, id)
	if err == nil && (row.ownerID == nil || *row.ownerID != member) {
		return false, savepoint.Commit(ctx)
	}
	if err == nil {
		err = reassigner.reassign(ctx, savepoint, id, receiver, row.version)
	}
	if err == nil {
		return true, savepoint.Commit(ctx)
	}
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		return false, fmt.Errorf("roll back the savepoint for %s: %w", id, rollbackErr)
	}
	if _, hard := bulkSkipFor(err); hard != nil {
		return false, hard
	}
	return false, nil
}

// handoverReceiver refuses an omitted to_user_id by name: it decodes to the
// zero UUID, which would otherwise read as a colleague who may not own records.
func handoverReceiver(body crmcontracts.HandOverUserWorkRequest) (ids.UUID, error) {
	receiver := ids.UUID(body.ToUserId)
	return receiver, httperr.RequireBodyID("to_user_id", receiver)
}

// handoverHandlers is the REST door onto the handover.
type handoverHandlers struct {
	seam handoverSeam
}

func (s *Server) wireHandoverSurface() {
	s.handoverHandlers = handoverHandlers{
		seam: newHandoverSeam(s.bulkHandlers.engine),
	}
}

func (h handoverHandlers) GetUserHandover(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	owned, err := h.seam.Owned(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.UserHandover{Owned: owned})
}

// HandOverUserWork needs nothing of Idempotency-Key itself: the middleware
// claims the key and replays the first answer to a retry.
func (h handoverHandlers) HandOverUserWork(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.HandOverUserWorkParams,
) {
	var body crmcontracts.HandOverUserWorkRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	receiver, err := handoverReceiver(body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.seam.HandOver(r.Context(), ids.UUID(id), receiver)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
