// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The three routes a share has: issue it, open it, close it.
//
// Opening is the one that carries the whole design. The caller is a colleague
// signed in as themselves, so the reading is computed under THEIR grants and
// not under the issuer's — a share widens who knows a link exists, never what
// anybody is allowed to read. What the token decides is WHICH view, and the
// issuer's standing decides whether it still serves at all.

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// analyticsShareHandlers serves the share routes.
type analyticsShareHandlers struct {
	shares   *AnalyticsShareStore
	forecast *forecasting.Store
	now      func() time.Time
}

// newAnalyticsShareHandlers binds the share store to the forecast store whose
// transaction every route runs in — that store's InTx is what gates the whole
// surface on forecast:read.
func newAnalyticsShareHandlers(
	shares *AnalyticsShareStore, forecast *forecasting.Store, now func() time.Time,
) analyticsShareHandlers {
	return analyticsShareHandlers{shares: shares, forecast: forecast, now: now}
}

// CreateForecastShare implements POST /forecast/shares.
func (h analyticsShareHandlers) CreateForecastShare(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.NewForecastShare
	if !httperr.Decode(w, r, &body) {
		return
	}
	in, err := shareFromBody(body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	var out crmcontracts.IssuedForecastShare
	if err := h.forecast.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		share, token, err := h.shares.Issue(ctx, tx, in)
		if err != nil {
			return err
		}
		out = issuedShareToWire(share, token)
		return nil
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// RevokeForecastShare implements DELETE /forecast/shares/{id}.
func (h analyticsShareHandlers) RevokeForecastShare(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	if err := h.forecast.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return h.shares.Revoke(ctx, tx, ids.UUID(id))
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// OpenForecastShare implements GET /forecast/shared/{token}.
//
// The reading is computed for whoever is signed in, not for whoever issued the
// link. A live share re-runs it; a snapshot share re-sums the frozen rows this
// caller may read.
func (h analyticsShareHandlers) OpenForecastShare(
	w http.ResponseWriter, r *http.Request, token string,
) {
	var out crmcontracts.SharedForecastView
	if err := h.forecast.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		share, err := h.shares.Resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		out.Kind = crmcontracts.SharedForecastViewKind(share.Kind)
		out.Target = share.Target

		if share.Kind == shareKindSnapshot {
			return h.serveSnapshot(ctx, tx, share, &out)
		}
		return h.serveLive(ctx, tx, share, &out)
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// serveSnapshot re-sums a frozen state over the rows this caller may read.
func (h analyticsShareHandlers) serveSnapshot(
	ctx context.Context, tx pgx.Tx, share Share, out *crmcontracts.SharedForecastView,
) error {
	if share.SnapshotID == nil {
		// The CHECK constraint and checkShareKind both hold this, so reaching
		// here means the row was written by something that bypassed them.
		// Refusing beats serving live data under a snapshot's name.
		return apperrors.ErrNotFound
	}
	shared, err := ReadSharedSnapshot(ctx, tx, *share.SnapshotID)
	if err != nil {
		return err
	}
	frame, err := snapshotFrame(ctx, tx, *share.SnapshotID)
	if err != nil {
		return err
	}
	// The frame comes from the SNAPSHOT and not from today's settings. A
	// reading is placed by its period, zone and currency, and re-deriving them
	// now would label a frozen number with a frame it was never computed under
	// — which is exactly what happens the first time an installation changes
	// its base currency or its fiscal year.
	out.AsOf = &frame.TakenAt
	out.Readings = forecasting.ReadingsToWire(
		frame.Period, share.Scope, shared.Readings, frame.BaseCurrency, frame.TakenAt)
	out.Withheld = shared.Withheld
	return nil
}

// serveLive re-runs the reading under this caller's grants.
func (h analyticsShareHandlers) serveLive(
	ctx context.Context, tx pgx.Tx, share Share, out *crmcontracts.SharedForecastView,
) error {
	at := h.now()
	period, baseCurrency, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
	if err != nil {
		return err
	}
	deals, limited, err := ForecastDeals(ctx, tx, period, share.Scope, at, baseCurrency)
	if err != nil {
		return err
	}
	readings, err := forecasting.Compute(period, period.LocalDay(at), deals)
	if err != nil {
		return err
	}
	out.Readings = forecasting.ReadingsToWire(period, share.Scope, readings, baseCurrency, at)
	out.Withheld = limited
	return nil
}

// snapshotFrame reads the period, zone and currency a frozen state was computed
// under, and the instant it describes.
//
// Read from the snapshot rather than carried on the share row: they are the
// snapshot's own facts, and copying them onto the share at issue time would be
// a second place for each of them to be wrong.
type shareFrame struct {
	Period       forecasting.Period
	BaseCurrency string
	TakenAt      time.Time
}

func snapshotFrame(ctx context.Context, tx pgx.Tx, id ids.UUID) (shareFrame, error) {
	var out shareFrame
	var zoneName string
	if err := tx.QueryRow(ctx, `
		SELECT s.period_start, s.period_end, s.base_currency, s.taken_at,
		       COALESCE(st.value #>> '{}', 'UTC')
		FROM forecast_snapshot s
		LEFT JOIN setting st ON st.key = 'installation.timezone'
		WHERE s.id = $1`, id).Scan(
		&out.Period.StartDate, &out.Period.EndDate, &out.BaseCurrency, &out.TakenAt,
		&zoneName); err != nil {
		return shareFrame{}, fmt.Errorf("compose: reading a shared snapshot's frame: %w", err)
	}
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		return shareFrame{}, fmt.Errorf(
			"compose: the installation zone %q is not a zone: %w", zoneName, err)
	}
	out.Period.Zone = zone
	return out, nil
}

// shareFromBody turns the request into what the store takes, refusing what the
// store would refuse anyway — here, so the caller gets an argument error rather
// than a database constraint surfacing as a 500.
func shareFromBody(in crmcontracts.NewForecastShare) (NewShare, error) {
	scope := forecasting.Scope{Kind: forecasting.ScopeWorkspace}
	if in.ScopeKind != nil {
		scope.Kind = string(*in.ScopeKind)
	}
	if in.ScopeId != nil {
		id := ids.UUID(*in.ScopeId)
		scope.ID = &id
	}
	// A workspace scope names no subject and every other kind names one.
	// Without this a team share with no id reads as a workspace share and shows
	// the whole pipeline to somebody who was sent one team's.
	if (scope.Kind == forecasting.ScopeWorkspace) != (scope.ID == nil) {
		return NewShare{}, fmt.Errorf(
			"%w: a workspace share names no scope_id and a team or owner share must",
			apperrors.ErrInvalidArgument)
	}
	out := NewShare{Kind: string(in.Kind), Target: in.Target, Scope: scope}
	if in.SnapshotId != nil {
		id := ids.UUID(*in.SnapshotId)
		out.SnapshotID = &id
	}
	if in.ExpiresAt != nil {
		out.ExpiresAt = *in.ExpiresAt
	}
	return out, nil
}

// issuedShareToWire renders the share with its token, which is the only moment
// the token exists outside the caller that asked for it.
func issuedShareToWire(in Share, token string) crmcontracts.IssuedForecastShare {
	scopeKind := crmcontracts.IssuedForecastShareScopeKind(in.Scope.Kind)
	out := crmcontracts.IssuedForecastShare{
		Id:        openapi_types.UUID(in.ID),
		Kind:      crmcontracts.IssuedForecastShareKind(in.Kind),
		Target:    in.Target,
		ScopeKind: &scopeKind,
		ExpiresAt: in.ExpiresAt,
		Token:     token,
		CreatedAt: in.CreatedAt,
	}
	if in.Scope.ID != nil {
		id := openapi_types.UUID(*in.Scope.ID)
		out.ScopeId = &id
	}
	if in.SnapshotID != nil {
		id := openapi_types.UUID(*in.SnapshotID)
		out.SnapshotId = &id
	}
	return out
}

// ExportForecastShare implements GET /forecast/shared/{token}/export.csv.
//
// The rows are the ones the headline was summed from, narrowed by the SAME
// predicate — sharedVisibilityClause, called by both. A file whose rows and
// whose total disagree gets reconciled by hand, and whoever does it concludes
// the product is wrong about one of them.
func (h analyticsShareHandlers) ExportForecastShare(
	w http.ResponseWriter, r *http.Request, token string,
) {
	var rows []SharedContribution
	if err := h.forecast.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		share, err := h.shares.Resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		if share.Kind != shareKindSnapshot || share.SnapshotID == nil {
			// A live share's rows are the deals as they stand, which the
			// forecast's own export already answers for a signed-in caller.
			// Refusing beats exporting a set assembled a second way here.
			return apperrors.ErrNotFound
		}
		rows, err = SharedSnapshotRows(ctx, tx, *share.SnapshotID)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}

	body, err := shareRowsCSV(rows)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.Download{
		ContentType: "text/csv; charset=utf-8",
		Filename:    "forecast-contributions.csv",
		Size:        int64(len(body)),
	}.WriteHeaders(w)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		// The body is fully rendered before the 200 goes out, so a failure
		// here is a broken client connection and not a fault this can still
		// answer. There is nothing left to say to the caller.
		return
	}
}

// shareExportColumns is the file's header, and its ORDER is the wire format:
// a spreadsheet somebody built a formula against breaks when a column moves.
//
// Spelled out here rather than borrowed from the field constants elsewhere in
// this package. Those name a request field or a database column; these name a
// column of an exported file, and tying the two together would mean renaming a
// field silently changed a file people have saved.
//
//nolint:goconst // these are COLUMN HEADINGS of an exported file, and the constants goconst points at name other concepts that spell the same word — a request field, a database column. Hiding these behind them would assert a correspondence that does not hold: renaming one of those would silently change a file people have already saved formulas against
var shareExportColumns = []string{
	"deal_id", "deal_name", "owner_id", "amount_minor", "currency",
	"base_minor", "effective_close_date", "category",
	"in_won", "in_open", "exclusion_reason",
}

// shareRowsCSV renders the rows, neutralising formula leads through the same
// csvCell every other export in this package uses.
func shareRowsCSV(rows []SharedContribution) ([]byte, error) {
	var buf bytes.Buffer
	out := csv.NewWriter(&buf)
	if err := out.Write(shareExportColumns); err != nil {
		return nil, fmt.Errorf("compose: writing the export header: %w", err)
	}
	for _, row := range rows {
		record := []string{
			csvCell(row.DealID), csvCell(row.DealName), csvCellPtr(row.Owner),
			csvCellPtr(row.AmountMinor), csvCell(row.Currency),
			csvCellPtr(row.BaseMinor), csvCellPtr(row.EffectiveClose),
			csvCell(row.Category), csvCell(row.InWon), csvCell(row.InOpen),
			csvCell(row.ExclusionReason),
		}
		if err := out.Write(record); err != nil {
			return nil, fmt.Errorf("compose: writing an export row: %w", err)
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return nil, fmt.Errorf("compose: finishing the export: %w", err)
	}
	return buf.Bytes(), nil
}

// csvCellPtr renders an optional value, with nil as an EMPTY cell rather than
// as a zero.
//
// The distinction is the whole reason the pointers exist: an unpriced deal has
// no amount, and a zero in that cell reads as a deal somebody expects nothing
// from — which is a different fact and the one this export must not invent.
func csvCellPtr[T any](v *T) string {
	if v == nil {
		return ""
	}
	return csvCell(*v)
}
