// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The four routes a share has: issue it, list your own, open it, close it.
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
	"strings"
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

// ListForecastShares implements GET /forecast/shares.
func (h analyticsShareHandlers) ListForecastShares(w http.ResponseWriter, r *http.Request) {
	var data []crmcontracts.ForecastShare
	if err := h.forecast.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		shares, err := h.shares.ListIssued(ctx, tx)
		if err != nil {
			return err
		}
		data = make([]crmcontracts.ForecastShare, 0, len(shares))
		for _, share := range shares {
			data = append(data, shareToWire(share))
		}
		return nil
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Data []crmcontracts.ForecastShare `json:"data"`
	}{Data: data})
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
		share, frozen, err := h.resolveForReader(ctx, tx, token)
		if err != nil {
			return err
		}
		out.Kind = crmcontracts.SharedForecastViewKind(share.Kind)
		out.Target = share.Target

		if share.Kind == shareKindSnapshot {
			return h.serveSnapshot(ctx, tx, share, frozen, &out)
		}
		return h.serveLive(ctx, tx, share, &out)
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// frozenForReader is what one reader may take from a frozen share: the rows
// listed to them, and the range over every row the headline sums. Limited says
// the rows are the reader's own while the headline covers the whole team.
type frozenForReader struct {
	Rows    []SharedContribution
	Range   *forecasting.Range
	Limited bool
}

// resolveForReader resolves a share for the signed-in reader. Both the frozen
// reading and its export pass it. A frozen share answers to the switch like a
// live read: its population, the owners behind its figures, and no colleague
// named on its rows. The range, like the totals, still counts colleagues' deals.
func (h analyticsShareHandlers) resolveForReader(
	ctx context.Context, tx pgx.Tx, token string,
) (Share, frozenForReader, error) {
	share, err := h.shares.Resolve(ctx, tx, token)
	if err != nil || share.Kind != shareKindSnapshot || share.SnapshotID == nil {
		return share, frozenForReader{}, err
	}
	rows, err := SharedSnapshotRows(ctx, tx, *share.SnapshotID)
	if err != nil {
		return Share{}, frozenForReader{}, err
	}
	out := frozenForReader{Range: sharedRange(rows)}
	if out.Rows, out.Limited, err = requireFrozenShareMeasurable(ctx, tx, share.Scope, rows); err != nil {
		return Share{}, frozenForReader{}, err
	}
	return share, out, nil
}

// serveSnapshot re-sums a frozen state over the rows this caller may read.
// frozen carries the range over those same rows and whether the export holds fewer.
func (h analyticsShareHandlers) serveSnapshot(
	ctx context.Context, tx pgx.Tx, share Share, frozen frozenForReader, out *crmcontracts.SharedForecastView,
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
	shared.Readings.Range = frozen.Range
	out.Readings = forecasting.ReadingsToWire(
		frame.Period, share.Scope, shared.Readings, frame.BaseCurrency, frame.TakenAt)
	out.Readings.SelfCorrecting = frame.SelfCorrecting
	out.Readings.ContributionsLimited = &frozen.Limited
	out.Withheld = shared.Withheld
	// A snapshot frozen before the landing was kept states none.
	if frame.LandingMeasure == nil {
		return nil
	}
	landing, err := frozenShareLanding(ctx, tx, share.Scope, shared.Readings, frame)
	out.Readings.Landing = &landing
	return err
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
	// A share carries the scope it was ISSUED with, which is already resolved;
	// the resolver hands it straight back. Taking the returned one anyway keeps
	// one answer to "which population" rather than two spellings that could
	// drift.
	assembly, err := ForecastAssemble(ctx, tx, period, share.Scope, at, baseCurrency)
	if err != nil {
		return err
	}
	share.Scope = assembly.Scope
	out.Readings = forecasting.ReadingsToWire(period, share.Scope, assembly.Readings, baseCurrency, at)
	out.Readings.SelfCorrecting = &assembly.SelfCorrecting
	out.Withheld = assembly.Limited
	// The landing GET /forecast gives this reader, with the call only where
	// the reader may see it.
	call, seen, err := forecastCallSeen(ctx, tx, h.forecast, period, share.Scope, false)
	if err != nil {
		return err
	}
	landing, err := forecasting.LandingFor(ctx, tx, ForecastForwardMeasure, assembly.Readings, seenCall(call, seen))
	if err != nil {
		return err
	}
	out.Readings.Landing = &landing
	return nil
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

// shareToWire renders a share without its token. A workspace share names no
// scope subject and a live one no snapshot, so both are absent rather than null.
func shareToWire(in Share) crmcontracts.ForecastShare {
	out := crmcontracts.ForecastShare{
		Id:        openapi_types.UUID(in.ID),
		Kind:      crmcontracts.ForecastShareKind(in.Kind),
		Target:    in.Target,
		ScopeKind: crmcontracts.ForecastShareScopeKind(in.Scope.Kind),
		ExpiresAt: in.ExpiresAt,
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

// issuedShareToWire renders the share with its token, which is the only moment
// the token exists outside the caller that asked for it.
func issuedShareToWire(in Share, token string) crmcontracts.IssuedForecastShare {
	listed := shareToWire(in)
	scopeKind := crmcontracts.IssuedForecastShareScopeKind(listed.ScopeKind)
	return crmcontracts.IssuedForecastShare{
		Id:         listed.Id,
		Kind:       crmcontracts.IssuedForecastShareKind(listed.Kind),
		Target:     listed.Target,
		ScopeKind:  &scopeKind,
		ScopeId:    listed.ScopeId,
		SnapshotId: listed.SnapshotId,
		ExpiresAt:  listed.ExpiresAt,
		CreatedAt:  listed.CreatedAt,
		Token:      token,
	}
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
	var frozen frozenForReader
	var frame shareFrame
	if err := h.forecast.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		share, read, err := h.resolveForReader(ctx, tx, token)
		if err != nil {
			return err
		}
		if share.Kind != shareKindSnapshot || share.SnapshotID == nil {
			// A live share's rows are the deals as they stand, which the
			// forecast's own export already answers for a signed-in caller.
			// Refusing beats exporting a set assembled a second way here.
			return apperrors.ErrNotFound
		}
		frozen = read
		frame, err = snapshotFrame(ctx, tx, *share.SnapshotID)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}

	body, err := shareRowsCSV(frozen, frame.SelfCorrecting)
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
// field silently changed a file contacts have saved.
//
//nolint:goconst // these are COLUMN HEADINGS of an exported file, and the constants goconst points at name other concepts that spell the same word — a request field, a database column. Hiding these behind them would assert a correspondence that does not hold: renaming one of those would silently change a file contacts have already saved formulas against
var shareExportColumns = []string{
	"deal_id", "deal_name", "owner_id", "amount_minor", "currency",
	"base_minor", "effective_close_date", "category",
	"in_won", "in_open", "exclusion_reason",
	"weighted_minor", "typed_probability", "chance_in_use", "evidence_n", "risk_signals", "risk_effect",
	"self_correcting", "risk_signal_facts", "range_low_minor", "range_high_minor",
	"contributions_limited",
}

// shareRowsCSV renders the rows, neutralising formula leads through the same
// csvCell every other export in this package uses.
// selfCorrecting is the snapshot's own, repeated on each row so a sheet filtered
// to some rows still says which definition they were frozen under. The range and
// the contributions_limited flag are the shared headline's, repeated the same way.
func shareRowsCSV(frozen frozenForReader, selfCorrecting *bool) ([]byte, error) {
	var low, high *int64
	if frozen.Range != nil {
		low, high = &frozen.Range.LowMinor, &frozen.Range.HighMinor
	}
	var buf bytes.Buffer
	out := csv.NewWriter(&buf)
	if err := out.Write(shareExportColumns); err != nil {
		return nil, fmt.Errorf("compose: writing the export header: %w", err)
	}
	for _, row := range frozen.Rows {
		record := []string{
			csvCell(row.DealID), csvCell(row.DealName), csvCellPtr(row.Owner),
			csvCellPtr(row.AmountMinor), csvCell(row.Currency),
			csvCellPtr(row.BaseMinor), csvCellPtr(row.EffectiveClose),
			csvCell(row.Category), csvCell(row.InWon), csvCell(row.InOpen),
			csvCell(row.ExclusionReason),
			csvCellPtr(row.WeightedMinor), csvCellPtr(row.TypedProbability), csvCellPtr(row.ChanceInUse),
			csvCellPtr(row.EvidenceN), csvCell(strings.Join(row.Signals, ";")), csvCellPtr(row.Effect),
			csvCellPtr(selfCorrecting), csvCellPtr(row.SignalFacts), csvCellPtr(low), csvCellPtr(high),
			csvCell(frozen.Limited),
		}
		if err := out.Write(record); err != nil {
			return nil, fmt.Errorf("compose: writing an export row: %w", err)
		}
	}
	if frozen.Limited && len(frozen.Rows) == 0 {
		if err := out.Write(shareSummaryRecord(selfCorrecting, low, high)); err != nil {
			return nil, fmt.Errorf("compose: writing the export's summary row: %w", err)
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return nil, fmt.Errorf("compose: finishing the export: %w", err)
	}
	return buf.Bytes(), nil
}

// shareSummaryRecord is the one row a file gets when every row was withheld
// from its reader. A header alone would not say the headline counts deals.
// Its deal cells are empty, and only the headline's own cells are filled.
func shareSummaryRecord(selfCorrecting *bool, low, high *int64) []string {
	record := make([]string, len(shareExportColumns))
	for index, column := range shareExportColumns {
		switch column {
		case "self_correcting":
			record[index] = csvCellPtr(selfCorrecting)
		case "range_low_minor":
			record[index] = csvCellPtr(low)
		case "range_high_minor":
			record[index] = csvCellPtr(high)
		case "contributions_limited":
			record[index] = csvCell(true)
		}
	}
	return record
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
