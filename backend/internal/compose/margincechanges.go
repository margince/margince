// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The report of what Margince changed on deals: the owner's open changes, one
// deal's list, a team's totals, and the Undo of many at once.
//
// Here rather than on the deals handler set because each answer is drawn as
// the receipts Home already shows, and those are rendered in compose.

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealChangesPage is how many changes one page of a deal's list carries.
const dealChangesPage = 20

// teamChangesWindow is how far back the team list reads when no `since` is
// given: one working week and its weekend.
const teamChangesWindow = 7 * 24 * time.Hour

type marginceChangeHandlers struct {
	pool      *pgxpool.Pool
	deals     *deals.Store
	teammates interface {
		LiveTeammatesOfCaller(ctx context.Context) ([]attention.TeamMember, bool, error)
	}
	now func() time.Time
}

func newMarginceChangeHandlers(pool *pgxpool.Pool) marginceChangeHandlers {
	return marginceChangeHandlers{
		pool:      pool,
		deals:     deals.NewStore(InstallationDB(pool), DealsInstallation()),
		teammates: newTeammatesSeam(pool),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// GetPendingMarginceChanges answers the reader's open changes, one bundle per deal.
func (h marginceChangeHandlers) GetPendingMarginceChanges(w http.ResponseWriter, r *http.Request) {
	summary, err := h.deals.PendingChangesOwnedBy(r.Context(), nil)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.MarginceChangeSummary{
		AsOf: h.now(), Deals: summary.Deals, Closed: summary.Closed, Redated: summary.Redated,
		Revalued: summary.Revalued, Probabilities: summary.Probabilities,
		BaseCurrency: summary.BaseCurrency, Truncated: summary.Truncated,
		Bundles: make([]crmcontracts.MarginceChangeBundle, 0, len(summary.Bundles)),
	}
	for _, b := range summary.Bundles {
		kinds := make([]crmcontracts.MarginceChangeBundleKinds, 0, len(b.Kinds))
		for _, kind := range b.Kinds {
			kinds = append(kinds, crmcontracts.MarginceChangeBundleKinds(kind))
		}
		out.Bundles = append(out.Bundles, crmcontracts.MarginceChangeBundle{
			DealId: openapi_types.UUID(b.DealID.UUID), DealName: b.DealName, Version: b.Version,
			AuditLogIds: wireUUIDs(b.AuditLogIDs), Kinds: kinds, ValueBaseMinor: b.ValueBaseMinor, LatestAt: b.LatestAt,
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListDealMarginceChanges answers one page of a deal's changes, drawn as the
// receipts Home shows.
func (h marginceChangeHandlers) ListDealMarginceChanges(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListDealMarginceChangesParams) {
	after, ok := decodeChangeCursor(params.Cursor)
	if !ok {
		httperr.Write(w, r, httperr.Validation("cursor", "invalid_cursor", "start again from the first page"))
		return
	}
	rows, more, err := h.deals.DealChanges(r.Context(), ids.From[ids.DealKind](ids.UUID(id)), after, dealChangesPage)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.DealMarginceChanges{Changes: make([]crmcontracts.Receipt, 0, len(rows))}
	for _, row := range rows {
		out.Changes = append(out.Changes, attention.WireReceipt(correctionReceipt(row)))
	}
	if more {
		last := rows[len(rows)-1]
		next := encodeChangeCursor(deals.ChangeCursor{AppliedAt: last.AppliedAt, AuditLogID: last.AuditLogID})
		out.NextCursor = &next
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetTeamMarginceChanges answers what Margince changed on the caller's team's
// deals, per teammate and kind.
func (h marginceChangeHandlers) GetTeamMarginceChanges(w http.ResponseWriter, r *http.Request, params crmcontracts.GetTeamMarginceChangesParams) {
	ctx := r.Context()
	if err := attention.RequireLeadTier(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	asOf := h.now()
	since := asOf.Add(-teamChangesWindow)
	if params.Since != nil {
		since = *params.Since
	}
	roster, cut, err := h.teammates.LiveTeammatesOfCaller(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	names := make(map[ids.UUID]string, len(roster))
	owners := make([]ids.UUID, 0, len(roster))
	for _, member := range roster {
		names[member.UserID] = member.DisplayName
		owners = append(owners, member.UserID)
	}
	report, err := h.deals.TeamChanges(ctx, owners, since)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.TeamMarginceChanges{
		AsOf: asOf, Since: since, BaseCurrency: report.BaseCurrency, Truncated: report.Truncated || cut,
		Groups: make([]crmcontracts.TeamMarginceChangeGroup, 0, len(report.Groups)),
		Rows:   make([]crmcontracts.TeamMarginceChangeRow, 0, len(report.Rows)),
	}
	for _, g := range report.Groups {
		out.Groups = append(out.Groups, crmcontracts.TeamMarginceChangeGroup{
			OwnerId: openapi_types.UUID(g.OwnerID), OwnerName: names[g.OwnerID], Kind: g.Kind,
			Deals: g.Deals, Undone: g.Undone, ValueBaseMinor: g.ValueBaseMinor,
		})
	}
	for _, row := range report.Rows {
		if row.OwnerID == nil {
			continue
		}
		out.Rows = append(out.Rows, crmcontracts.TeamMarginceChangeRow{
			OwnerId: openapi_types.UUID(*row.OwnerID), Change: attention.WireReceipt(correctionReceipt(row)),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// UndoMarginceChanges takes back the named changes, deal by deal.
func (h marginceChangeHandlers) UndoMarginceChanges(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.UndoMarginceChangesRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out := crmcontracts.UndoMarginceChangesResult{Results: make([]crmcontracts.UndoMarginceChangeResult, 0, len(req.Items))}
	for _, item := range req.Items {
		result, err := h.undoBundle(r.Context(), item)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		out.Results = append(out.Results, result)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// undoBundle takes back one deal's changes, newest first, each in its own
// transaction through the one reversal the single Undo uses. The caller's
// version guards the first; each later one is checked against the version the
// one before it left.
func (h marginceChangeHandlers) undoBundle(ctx context.Context, item crmcontracts.UndoMarginceChangeItem) (crmcontracts.UndoMarginceChangeResult, error) {
	dealID := ids.UUID(item.DealId)
	result := crmcontracts.UndoMarginceChangeResult{
		DealId: item.DealId, Undone: []string{}, Kept: []crmcontracts.UndoMarginceChangeKept{},
	}
	corrections, err := h.bundleCorrections(ctx, dealID, item.AuditLogIds, &result)
	if err != nil {
		return result, err
	}
	version := item.Version
	for i, correction := range corrections {
		if correction.Reversed() {
			result.Kept = append(result.Kept, keptChange(correction, crmcontracts.UndoMarginceChangeKeptReasonAlreadyUndone))
			continue
		}
		deal, err := h.deals.RevertCorrection(ctx, correction.ID, version,
			map[string]any{undidAuditLogID: correction.AuditLogID.String()})
		reason, refused := undoRefusal(err)
		if !refused && err != nil {
			return result, err
		}
		if reason == crmcontracts.UndoMarginceChangeKeptReasonVersionSkew {
			// The deal moved since the caller read it; every later change would
			// be checked against a version nobody saw.
			for _, rest := range corrections[i:] {
				result.Kept = append(result.Kept, keptChange(rest, reason))
			}
			break
		}
		if refused {
			result.Kept = append(result.Kept, keptChange(correction, reason))
			continue
		}
		result.Undone = append(result.Undone, correction.Correction)
		version = deal.Version
	}
	switch {
	case len(result.Kept) == 0:
		result.Outcome = crmcontracts.UndoMarginceChangeResultOutcomeUndone
	case len(result.Undone) == 0:
		result.Outcome = crmcontracts.UndoMarginceChangeResultOutcomeRefused
	default:
		result.Outcome = crmcontracts.UndoMarginceChangeResultOutcomePartlyUndone
	}
	return result, nil
}

// bundleCorrections reads the named changes, newest first. A change the reader
// may not see, or one on another deal, is kept as not found.
func (h marginceChangeHandlers) bundleCorrections(
	ctx context.Context, dealID ids.UUID, auditIDs []openapi_types.UUID, result *crmcontracts.UndoMarginceChangeResult,
) ([]deals.DealCorrection, error) {
	var found []deals.DealCorrection
	for _, auditID := range auditIDs {
		var correction deals.DealCorrection
		err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
			var err error
			correction, err = h.deals.CorrectionForAudit(ctx, tx, ids.UUID(auditID))
			return err
		})
		reason, refused := undoRefusal(err)
		if !refused && err != nil {
			return nil, err
		}
		if refused || correction.DealID.UUID != dealID {
			if !refused {
				reason = crmcontracts.UndoMarginceChangeKeptReasonNotFound
			}
			result.Kept = append(result.Kept, crmcontracts.UndoMarginceChangeKept{AuditLogId: auditID, Reason: reason})
			continue
		}
		found = append(found, correction)
	}
	slices.SortFunc(found, func(a, b deals.DealCorrection) int {
		if c := b.AppliedAt.Compare(a.AppliedAt); c != 0 {
			return c
		}
		return strings.Compare(b.AuditLogID.String(), a.AuditLogID.String())
	})
	return found, nil
}

// undoRefusal names a refusal the result reports per change. Any other error
// is the request's and fails it.
func undoRefusal(err error) (crmcontracts.UndoMarginceChangeKeptReason, bool) {
	if err == nil {
		return "", false
	}
	if _, ok := errors.AsType[*deals.CorrectionReversalError](err); ok {
		return crmcontracts.UndoMarginceChangeKeptReasonChangedSince, true
	}
	switch {
	case errors.Is(err, apperrors.ErrVersionSkew):
		return crmcontracts.UndoMarginceChangeKeptReasonVersionSkew, true
	case errors.Is(err, apperrors.ErrNotFound):
		return crmcontracts.UndoMarginceChangeKeptReasonNotFound, true
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return crmcontracts.UndoMarginceChangeKeptReasonNotPermitted, true
	}
	return "", false
}

func keptChange(c deals.DealCorrection, reason crmcontracts.UndoMarginceChangeKeptReason) crmcontracts.UndoMarginceChangeKept {
	kind := c.Correction
	return crmcontracts.UndoMarginceChangeKept{AuditLogId: openapi_types.UUID(c.AuditLogID), Kind: &kind, Reason: reason}
}

func wireUUIDs(in []ids.UUID) []openapi_types.UUID {
	out := make([]openapi_types.UUID, 0, len(in))
	for _, id := range in {
		out = append(out, openapi_types.UUID(id))
	}
	return out
}

// encodeChangeCursor and decodeChangeCursor carry where a deal's list stopped.
func encodeChangeCursor(c deals.ChangeCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.AppliedAt.UTC().Format(time.RFC3339Nano) + "|" + c.AuditLogID.String()))
}

func decodeChangeCursor(raw *string) (*deals.ChangeCursor, bool) {
	if raw == nil || *raw == "" {
		return nil, true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*raw)
	if err != nil {
		return nil, false
	}
	at, id, found := strings.Cut(string(decoded), "|")
	if !found {
		return nil, false
	}
	appliedAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, false
	}
	auditID, err := ids.Parse(id)
	if err != nil {
		return nil, false
	}
	return &deals.ChangeCursor{AppliedAt: appliedAt, AuditLogID: auditID}, true
}
