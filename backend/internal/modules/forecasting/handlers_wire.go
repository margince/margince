// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReadingsToWire is exported so a share can render the same envelope a direct
// read renders. A second converter would be a second answer to what a reading
// looks like on the wire. The two would drift the first time a field was added.
func ReadingsToWire(
	period Period, scope Scope, in Readings, baseCurrency string, asOf time.Time,
) crmcontracts.ForecastReadings {
	out := crmcontracts.ForecastReadings{
		PeriodStart:        openapi_types.Date{Time: period.StartDate},
		PeriodEnd:          openapi_types.Date{Time: period.EndDate},
		ScopeKind:          crmcontracts.ForecastReadingsScopeKind(scope.Kind),
		WonMinor:           in.WonMinor,
		EvidenceMinor:      in.EvidenceMinor,
		BestCaseMinor:      in.BestCaseMinor,
		OpenMinor:          in.OpenMinor,
		WeightedMinor:      in.WeightedMinor,
		EligibleCount:      in.EligibleCount,
		PricedCount:        in.PricedCount,
		ConfirmedDateCount: in.ConfirmedDateCount,
		FxMissingCount:     in.FxMissingCount,
		AsOf:               asOf,
		Timezone:           period.Zone.String(),
		BaseCurrency:       baseCurrency,
	}
	if scope.ID != nil {
		id := openapi_types.UUID(*scope.ID)
		out.ScopeId = &id
	}
	return out
}

func callToWire(in Call) crmcontracts.ForecastCall {
	out := crmcontracts.ForecastCall{
		Id:          openapi_types.UUID(in.ID),
		PeriodStart: openapi_types.Date{Time: in.PeriodStart},
		PeriodEnd:   openapi_types.Date{Time: in.PeriodEnd},
		ScopeKind:   crmcontracts.ForecastCallScopeKind(in.Scope.Kind),
		AmountMinor: in.AmountMinor,
		Currency:    in.Currency,
		AuthorId:    openapi_types.UUID(in.AuthorID),
		CreatedAt:   in.CreatedAt,
	}
	if in.Scope.ID != nil {
		id := openapi_types.UUID(*in.Scope.ID)
		out.ScopeId = &id
	}
	if in.Note != "" {
		out.Note = &in.Note
	}
	if in.SupersedesID != nil {
		id := openapi_types.UUID(*in.SupersedesID)
		out.SupersedesId = &id
	}
	return out
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetForecastMovement answers what moved between two snapshots.
func (h Handlers) GetForecastMovement(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetForecastMovementParams,
) {
	reading := ReadingOpen
	if params.Reading != nil {
		reading = Reading(*params.Reading)
	}
	out, err := h.store.Movement(r.Context(), reading,
		ids.UUID(params.From), ids.UUID(params.To))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, movementToWire(reading, out))
}

func movementToWire(reading Reading, in Movement) crmcontracts.ForecastMovement {
	out := crmcontracts.ForecastMovement{
		Reading:      crmcontracts.ForecastMovementReading(reading),
		OpeningMinor: in.OpeningMinor,
		ClosingMinor: in.ClosingMinor,
		// Empty, never nil. "Nothing moved" is a real answer and arrives shaped
		// like the array it is; nil marshals to null, which a reader takes for
		// "unknown".
		Buckets: []crmcontracts.ForecastMovementBucket{},
		Deals:   []crmcontracts.ForecastMovementDeal{},
	}
	for _, b := range in.Buckets {
		out.Buckets = append(out.Buckets, crmcontracts.ForecastMovementBucket{
			Name:        crmcontracts.ForecastMovementBucketName(b.Name),
			AmountMinor: b.AmountMinor,
			DealCount:   b.DealCount,
		})
	}
	for _, d := range in.Deals {
		wire := crmcontracts.ForecastMovementDeal{
			Bucket:      d.Bucket,
			AmountMinor: d.AmountMinor,
			FromMinor:   d.FromMinor,
			ToMinor:     d.ToMinor,
		}
		if parsed, err := ids.Parse(d.DealID); err == nil {
			wire.DealId = openapi_types.UUID(parsed)
		}
		if d.AuditID != nil {
			id := openapi_types.UUID(*d.AuditID)
			wire.AuditId = &id
		}
		if d.ApprovalID != nil {
			id := openapi_types.UUID(*d.ApprovalID)
			wire.ApprovalId = &id
		}
		out.Deals = append(out.Deals, wire)
	}
	return out
}
