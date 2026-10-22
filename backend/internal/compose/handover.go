// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The handover of a member's work: every live record they own moves to a named
// colleague through that record's own owner-change write, so each record gets
// the audit row, event and follow-on moves (a deal's pending proposals) one
// reassignment writes. Deactivating a member moves nothing, so this is how the
// work a leaver owned gets a new owner.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
// each record exactly as a bulk reassignment of it would.
type handoverSeam struct {
	db       *database.DB
	targets  map[crmcontracts.BulkRecordType]bulkTarget
	projects *projects.Store
	members  *identity.Service
}

func newHandoverSeam(engine *bulkEngine, projectsStore *projects.Store, members *identity.Service) handoverSeam {
	return handoverSeam{db: engine.db, targets: engine.targets, projects: projectsStore, members: members}
}

// admit is the gate both reads and writes pass: a human whom the member
// administration verbs would admit to act on this member. Every record moved
// after it is still gated by that record's own write check.
func (s handoverSeam) admit(ctx context.Context, member ids.UUID, action principal.Action) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	return s.members.AdmitMemberAdministration(ctx, ids.From[ids.UserKind](member), action)
}

// Owned counts the live records member owns that the caller can see.
func (s handoverSeam) Owned(ctx context.Context, member ids.UUID) (crmcontracts.HandoverCounts, error) {
	if err := s.admit(ctx, member, principal.ActionRead); err != nil {
		return crmcontracts.HandoverCounts{}, err
	}
	return s.owned(ctx, member)
}

func (s handoverSeam) owned(ctx context.Context, member ids.UUID) (crmcontracts.HandoverCounts, error) {
	var counts crmcontracts.HandoverCounts
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for table, count := range handoverTally(&counts) {
			owned, err := visibleOwnedIDs(ctx, tx, table, member)
			if err != nil {
				return err
			}
			*count = len(owned)
		}
		return nil
	})
	return counts, err
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
		"lead": &counts.Lead, "contact": &counts.Contact, "company": &counts.Company,
		"deal": &counts.Deal, "project": &counts.Project,
	}
}

// HandOver moves every live record member owns that the caller may change to
// receiver, and answers what moved and what member still owns.
func (s handoverSeam) HandOver(ctx context.Context, member, receiver ids.UUID) (crmcontracts.UserHandoverResult, error) {
	if err := s.admit(ctx, member, principal.ActionUpdate); err != nil {
		return crmcontracts.UserHandoverResult{}, err
	}
	if member == receiver {
		return crmcontracts.UserHandoverResult{}, &handoverSameUserError{}
	}
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error { return auth.EnsureAssignee(ctx, tx, receiver) }); err != nil {
		if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			return crmcontracts.UserHandoverResult{}, &handoverReceiverError{}
		}
		return crmcontracts.UserHandoverResult{}, err
	}
	var moved crmcontracts.HandoverCounts
	tally := handoverTally(&moved)
	for _, kind := range handoverRecordKinds {
		n, err := s.handOverKind(ctx, kind, member, ids.From[ids.UserKind](receiver))
		if err != nil {
			return crmcontracts.UserHandoverResult{}, err
		}
		*tally[string(kind)] = n
	}
	projectsMoved, err := s.projects.TransferProjectOwnership(ctx, projects.TransferProjectOwnershipInput{
		FromOwnerID: ids.From[ids.UserKind](member), ToOwnerID: ids.From[ids.UserKind](receiver),
	})
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return crmcontracts.UserHandoverResult{}, err
	}
	moved.Project = projectsMoved
	left, err := s.owned(ctx, member)
	return crmcontracts.UserHandoverResult{Moved: moved, Left: left}, err
}

// handOverKind moves one record type in one transaction, each row in its own
// savepoint: a row its own write refuses stays with the member, and an error
// that is no refusal aborts the type.
func (s handoverSeam) handOverKind(
	ctx context.Context, kind crmcontracts.BulkRecordType, member ids.UUID, receiver ids.UserID,
) (int, error) {
	target := s.targets[kind]
	reassigner, err := reassignerOf(target)
	if err != nil {
		return 0, err
	}
	moved := 0
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		owned, err := visibleOwnedIDs(ctx, tx, string(kind), member)
		if err != nil {
			return err
		}
		for _, id := range owned {
			ok, err := handOverRow(ctx, tx, target, reassigner, id, member, receiver)
			if err != nil {
				return err
			}
			if ok {
				moved++
			}
		}
		return nil
	})
	return moved, err
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

// handoverHandlers is the REST door onto the handover.
type handoverHandlers struct {
	seam handoverSeam
}

func (s *Server) wireHandoverSurface(pool *pgxpool.Pool) {
	s.handoverHandlers = handoverHandlers{
		seam: newHandoverSeam(s.bulkHandlers.engine, ProjectsStoreOver(InstallationDB(pool)), identity.NewService(pool)),
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
	out, err := h.seam.HandOver(r.Context(), ids.UUID(id), ids.UUID(body.ToUserId))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
