// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type fieldFaulted interface {
	FieldFault() (field, code, message string)
}

func faultOf(t *testing.T, err error) (field, code string) {
	t.Helper()
	var f fieldFaulted
	if !errors.As(err, &f) {
		t.Fatalf("%v does not name a field", err)
	}
	field, code, _ = f.FieldFault()
	return field, code
}

func TestAPatchNamingAClosingFieldIsRefusedOnThatField(t *testing.T) {
	status := crmcontracts.UpdateDealRequestStatus("lost")
	reason, rate := "price", "1.1"
	date := openapi_types.Date{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	for field, req := range map[string]crmcontracts.UpdateDealRequest{
		"status":          {Status: &status},
		"lost_reason":     {LostReason: &reason},
		"fx_rate_to_base": {FxRateToBase: &rate},
		"fx_rate_date":    {FxRateDate: &date},
	} {
		got, code := faultOf(t, refuseClosingFields(req))
		if got != field || code != "set_by_advance" {
			t.Errorf("a patch carrying %s was refused on %s/%s", field, got, code)
		}
	}
	name := "Renamed"
	if err := refuseClosingFields(crmcontracts.UpdateDealRequest{Name: &name}); err != nil {
		t.Errorf("an ordinary patch was refused: %v", err)
	}
}

func TestAnOfferPastItsDateIsRefusedFromTheDayAfter(t *testing.T) {
	on := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	today := on(2026, 10, 8)
	for name, tc := range map[string]struct {
		validUntil time.Time
		lapsed     bool
	}{
		"today":              {on(2026, 10, 8), false},
		"tomorrow":           {on(2026, 10, 9), false},
		"yesterday":          {on(2026, 10, 7), true},
		"years ago":          {on(2020, 1, 1), true},
		"first of the month": {on(2026, 10, 1), true},
	} {
		err := lapsedOn(tc.validUntil, today)
		if (err != nil) != tc.lapsed {
			t.Errorf("%s: refused = %v, want %v", name, err != nil, tc.lapsed)
			continue
		}
		if err != nil {
			if field, code := faultOf(t, err); field != "valid_until" || code != "offer_lapsed" {
				t.Errorf("%s: refused on %s/%s", name, field, code)
			}
		}
	}
}

func TestAnOfferWithoutADateIsNeverLapsed(t *testing.T) {
	if err := (&Store{}).refuseLapsedOffer(context.Background(), nil, crmcontracts.Offer{}); err != nil {
		t.Errorf("an offer with no valid_until was refused: %v", err)
	}
}

func TestAStageMismatchNamesTheFieldTheCallerSent(t *testing.T) {
	if field, _ := faultOf(t, &StagePipelineMismatchError{Field: "stage_id"}); field != "stage_id" {
		t.Errorf("a create-time mismatch names %s, want stage_id", field)
	}
	if field, _ := faultOf(t, &StagePipelineMismatchError{}); field != "to_stage_id" {
		t.Errorf("an advance-time mismatch names %s, want to_stage_id", field)
	}
}

func TestAnAgentUpdateNamingTheClosingFieldsIsRefusedBeforeAnyWrite(t *testing.T) {
	provider := &Provider{}
	_, err := provider.Update(context.Background(), datasource.UpdateInput{
		Ref:   datasource.EntityRef{Type: datasource.EntityDeal},
		Patch: map[string]any{"status": "lost", "lost_reason": "price"},
	})
	if field, code := faultOf(t, err); field != "status" || code != "set_by_advance" {
		t.Errorf("an agent patch carrying status was refused on %s/%s", field, code)
	}
}
