// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestEveryRecurringWriterNamesAnUnsupportedBillingInterval(t *testing.T) {
	for _, months := range []int{-1, 0, 1, 2, 3, 4, 5, 6, 12, 13, 1000000} {
		expected := crmcontracts.ProductBillingIntervalMonths(months).Valid()
		if expected != crmcontracts.OfferLineItemInputBillingIntervalMonths(months).Valid() || expected != crmcontracts.AddDealLineRequestBillingIntervalMonths(months).Valid() {
			t.Fatalf("billing schemas disagree at %d", months)
		}
		err := validBillingShape(new(BillingRecurring), &months)
		if expected {
			if err != nil {
				t.Errorf("supported interval %d: %v", months, err)
			}
			continue
		}
		var field *BillingShapeError
		if !errors.As(err, &field) || field.Field != "billing_interval_months" {
			t.Fatalf("interval %d gave %v", months, err)
		}
		rec := httptest.NewRecorder()
		httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/v1/products", nil), err)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"billing_interval_months"`) {
			t.Fatalf("interval %d gave %d %s", months, rec.Code, rec.Body)
		}
	}
}
