// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A seller can ask the team's manager to look at a deal. The request is one
// fact on the deal row, so every lead of the owner's teams sees it and none
// has a copy to keep in step. The write is the deal's own: row, audit entry and
// event commit together.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	managerRequestedAt      = "manager_requested_at"
	managerRequestedBy      = "manager_requested_by"
	managerRequestedReasons = "manager_request_reasons"
)

// ManagerRequestReasons are the reasons a request may name, in the order the
// move lists them.
var ManagerRequestReasons = []string{"big_deal", "single_contact", "stuck"}

// ManagerRequest is one deal a seller asked the manager to look at.
type ManagerRequest struct {
	DealID      ids.UUID
	Name        string
	OwnerID     *ids.UUID
	RequestedBy *ids.UUID
	RequestedAt time.Time
	Reasons     []string
}

// RequestManager records that the caller asks the team's manager to look at an
// open deal. Asking again with the same reasons changes nothing. Reasons that
// differ replace the earlier ones, because the deal then qualifies differently.
func (s *Store) RequestManager(ctx context.Context, id ids.DealID, reasons []string) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, dealTable, principal.ActionUpdate); err != nil {
		return err
	}
	reasons, err := checkedReasons(reasons)
	if err != nil {
		return err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockThenEnsureWritable(ctx, tx, id); err != nil {
			return err
		}
		var open bool
		var at *time.Time
		var by *ids.UUID
		var before []string
		if err := tx.QueryRow(ctx, `SELECT status = 'open', manager_requested_at, manager_requested_by, manager_request_reasons
			FROM deal WHERE id = $1 AND archived_at IS NULL`, id).Scan(&open, &at, &by, &before); err != nil {
			return noRowIsNotFound(err)
		}
		if !open {
			return apperrors.ErrNotFound
		}
		if at != nil && slices.Equal(before, reasons) {
			return nil
		}
		now := time.Now().UTC()
		p := storekit.NewPatch()
		p.Set(managerRequestedAt, at, now)
		p.Set(managerRequestedBy, by, actor.UserID)
		p.Set(managerRequestedReasons, before, reasons)
		return s.writeManagerRequest(ctx, tx, id, p)
	})
}

// ClearManagerRequest marks a deal's request handled. The caller must lead a
// team or hold oversight of every team, and may change the deal. A deal with no
// request is left as it is.
func (s *Store) ClearManagerRequest(ctx context.Context, id ids.DealID) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, dealTable, principal.ActionUpdate); err != nil {
		return err
	}
	if auth.TeamLeadReachOf(ctx) == auth.ReachesNoTeam {
		return fmt.Errorf("clearing a manager request: %w", apperrors.ErrPermissionDenied)
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockThenEnsureWritable(ctx, tx, id); err != nil {
			return err
		}
		var at *time.Time
		var by *ids.UUID
		var before []string
		if err := tx.QueryRow(ctx, `SELECT manager_requested_at, manager_requested_by, manager_request_reasons
			FROM deal WHERE id = $1 AND archived_at IS NULL`, id).Scan(&at, &by, &before); err != nil {
			return noRowIsNotFound(err)
		}
		if at == nil {
			return nil
		}
		p := storekit.NewPatch()
		p.Set(managerRequestedAt, at, nil)
		p.Set(managerRequestedBy, by, nil)
		p.Set(managerRequestedReasons, before, nil)
		return s.writeManagerRequest(ctx, tx, id, p)
	})
}

func (s *Store) writeManagerRequest(ctx context.Context, tx pgx.Tx, id ids.DealID, p *storekit.Patch) error {
	if err := applyDealPatchGuarded(ctx, tx, id, p, nil); err != nil {
		return fmt.Errorf("write the manager request: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", dealTable, id.UUID, p.Before(), p.After())
	if err != nil {
		return fmt.Errorf("audit the manager request: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: p.After()})
}

func noRowIsNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	return err
}

func checkedReasons(reasons []string) ([]string, error) {
	out := make([]string, 0, len(reasons))
	for _, reason := range ManagerRequestReasons {
		if slices.Contains(reasons, reason) {
			out = append(out, reason)
		}
	}
	unknown := slices.ContainsFunc(reasons, func(reason string) bool { return !slices.Contains(ManagerRequestReasons, reason) })
	if len(out) == 0 || unknown {
		return nil, httperr.Validation("reasons", "unknown", "name why the manager is asked: big_deal, single_contact or stuck")
	}
	return out, nil
}

// ManagerRequests lists the open deals the caller may read that carry a
// request, oldest first. Only a team lead, or a seat with oversight of every
// team, reads them. team narrows to deals owned by that team's members.
func (s *Store) ManagerRequests(ctx context.Context, tx pgx.Tx, team *ids.UUID) ([]ManagerRequest, error) {
	if auth.TeamLeadReachOf(ctx) == auth.ReachesNoTeam {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := "d.manager_requested_at IS NOT NULL AND d.archived_at IS NULL AND d.status = 'open'"
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		where += " AND " + scope
	}
	if team != nil {
		where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM team_membership m WHERE m.team_id = $%d AND m.user_id = d.owner_id)", arg(*team))
	}
	rows, err := tx.Query(ctx, `SELECT d.id, d.name, d.owner_id, d.manager_requested_by, d.manager_requested_at, d.manager_request_reasons
		FROM deal d WHERE `+where+` ORDER BY d.manager_requested_at, d.id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ManagerRequest, error) {
		var out ManagerRequest
		err := row.Scan(&out.DealID, &out.Name, &out.OwnerID, &out.RequestedBy, &out.RequestedAt, &out.Reasons)
		return out, err
	})
}

// RequestDealManager implements POST /deals/{id}/manager-request.
func (h Handlers) RequestDealManager(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.RequestDealManagerRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	reasons := make([]string, len(body.Reasons))
	for i, reason := range body.Reasons {
		reasons[i] = string(reason)
	}
	if err := h.store.RequestManager(r.Context(), pathID[ids.DealKind](id), reasons); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ClearDealManagerRequest implements DELETE /deals/{id}/manager-request.
func (h Handlers) ClearDealManagerRequest(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.ClearManagerRequest(r.Context(), pathID[ids.DealKind](id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ManagerRequestedAmongTx answers which of these deals already carry a request,
// among the ones the caller may read.
func ManagerRequestedAmongTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	if len(dealIDs) == 0 {
		return out, nil
	}
	args := []any{dealIDs}
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := "d.id = ANY($1) AND d.manager_requested_at IS NOT NULL"
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		where += " AND " + scope
	}
	rows, err := tx.Query(ctx, `SELECT d.id FROM deal d WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	for _, id := range found {
		out[id] = true
	}
	return out, err
}
