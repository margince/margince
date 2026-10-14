// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The product editor restates the billing pair on every save, with a null
// interval for any price that does not repeat. That null belongs to the pair,
// so the save lands; a null on one half with no model beside it is still
// refused by name.
func TestAProductEditRestatesItsBillingPairWithANullInterval(t *testing.T) {
	e := Setup(t)
	editor := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"deal_desk"},
		Objects: map[string]principal.ObjectGrant{
			"product": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})
	handlers := deals.NewHandlers(e.DB(), installseam.Deals())

	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantModel  string
		wantMonths int
		wantFault  string
	}{
		{
			name:       "unclassified stays unclassified",
			body:       `{"name":"Workshop","billing_model":"not_specified","billing_interval_months":null}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "one-off with no cadence",
			body:       `{"name":"Workshop","billing_model":"one_time","billing_interval_months":null}`,
			wantStatus: http.StatusOK,
			wantModel:  "one_time",
		},
		{
			name:       "recurring with its cadence",
			body:       `{"name":"Workshop","billing_model":"recurring","billing_interval_months":12}`,
			wantStatus: http.StatusOK,
			wantModel:  "recurring",
			wantMonths: 12,
		},
		{
			name:       "recurring without a cadence is the shape refusal",
			body:       `{"billing_model":"recurring","billing_interval_months":null}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantFault:  "billing_interval_months/billing_shape",
		},
		{
			name:       "a lone null interval is half a pair",
			body:       `{"billing_interval_months":null}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantFault:  "billing_interval_months/field_not_clearable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := e.Deals.CreateProduct(editor, deals.CreateProductInput{
				Name: "Workshop", UnitPriceMinor: 120_000, Currency: "EUR", Source: "manual",
			})
			if err != nil {
				t.Fatalf("create an unclassified product: %v", err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/v1/products/"+created.Id.String(),
				strings.NewReader(tc.body)).WithContext(editor)
			req.Header.Set("If-Match", strconv.FormatInt(*created.Version, 10))
			handlers.UpdateProduct(rec, req, created.Id, crmcontracts.UpdateProductParams{})

			if rec.Code != tc.wantStatus {
				t.Fatalf("PATCH %s answered %d, want %d: %s", tc.body, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				assertRefusalNames(t, rec.Body.Bytes(), tc.wantFault)
				return
			}
			var product crmcontracts.Product
			if err := json.Unmarshal(rec.Body.Bytes(), &product); err != nil {
				t.Fatalf("decode the product: %v", err)
			}
			if got := billingModelOf(product); got != tc.wantModel {
				t.Errorf("billing_model = %q, want %q", got, tc.wantModel)
			}
			if got := billingMonthsOf(product); got != tc.wantMonths {
				t.Errorf("billing_interval_months = %d, want %d", got, tc.wantMonths)
			}
		})
	}
}

// assertRefusalNames finds fault, spelled field/code, in a 422's field list.
func assertRefusalNames(t *testing.T, body []byte, fault string) {
	t.Helper()
	var problem struct {
		Details struct {
			Errors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("decode the refusal: %v", err)
	}
	for _, named := range problem.Details.Errors {
		if named.Field+"/"+named.Code == fault {
			return
		}
	}
	t.Errorf("the refusal %s does not carry %q", body, fault)
}

func billingModelOf(p crmcontracts.Product) string {
	if p.BillingModel == nil {
		return ""
	}
	return string(*p.BillingModel)
}

func billingMonthsOf(p crmcontracts.Product) int {
	if p.BillingIntervalMonths == nil {
		return 0
	}
	return int(*p.BillingIntervalMonths)
}
