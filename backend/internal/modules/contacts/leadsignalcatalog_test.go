// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheCatalogFactorsAreTheContractFactors(t *testing.T) {
	want := []string{
		string(crmcontracts.LeadManualSignalFactorWebTraffic),
		string(crmcontracts.LeadManualSignalFactorEmployees),
		string(crmcontracts.LeadManualSignalFactorBudgetHint),
	}
	var got []string
	for _, f := range LeadSignalCatalog() {
		got = append(got, f.ID)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("catalog factors %v, contract factors %v", got, want)
	}
}

func TestEveryCompanySizeBandMapsToExactlyOneEmployeesBand(t *testing.T) {
	sizes := []crmcontracts.CompanySizeBand{
		crmcontracts.CompanySizeBandN110, crmcontracts.CompanySizeBandN1150, crmcontracts.CompanySizeBandN51200,
		crmcontracts.CompanySizeBandN201500, crmcontracts.CompanySizeBandN5011000,
		crmcontracts.CompanySizeBandN10015000, crmcontracts.CompanySizeBandN5000,
	}
	owners := map[string]int{}
	for _, f := range LeadSignalCatalog() {
		for _, b := range f.Bands {
			if f.CompanyField == "" && len(b.CompanySizeBands) > 0 {
				t.Errorf("%s/%s maps company sizes but the factor names no company field", f.ID, b.ID)
			}
			for _, size := range b.CompanySizeBands {
				owners[size]++
			}
		}
	}
	for _, size := range sizes {
		if owners[string(size)] != 1 {
			t.Errorf("company size band %q is covered by %d employees bands, want 1", size, owners[string(size)])
		}
	}
	if len(owners) != len(sizes) {
		t.Errorf("catalog maps %d sizes, the company contract has %d", len(owners), len(sizes))
	}
}

func TestTheStoreValidatesAgainstTheCatalog(t *testing.T) {
	for _, f := range LeadSignalCatalog() {
		for _, b := range f.Bands {
			points, known, valid := leadSignalPoints(f.ID, b.ID)
			if !known || !valid || points != b.Points {
				t.Errorf("%s/%s resolved to %d known=%v valid=%v", f.ID, b.ID, points, known, valid)
			}
		}
		if _, known, valid := leadSignalPoints(f.ID, "nonsense"); !known || valid {
			t.Errorf("%s accepted an unknown band", f.ID)
		}
	}
	if _, known, _ := leadSignalPoints("nonsense", "low"); known {
		t.Error("an unknown factor was accepted")
	}
}

func TestTheCatalogEndpointServesTheStoresCatalog(t *testing.T) {
	rec := httptest.NewRecorder()
	Handlers{}.GetLeadSignalCatalog(rec, httptest.NewRequest(http.MethodGet, "/v1/lead-signal-catalog", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got crmcontracts.LeadSignalCatalog
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	catalog := LeadSignalCatalog()
	if len(got.Factors) != len(catalog) {
		t.Fatalf("served %d factors, store has %d", len(got.Factors), len(catalog))
	}
	for i, f := range catalog {
		served := got.Factors[i]
		if string(served.Factor) != f.ID || len(served.Bands) != len(f.Bands) {
			t.Fatalf("factor %d served as %+v, want %s with %d bands", i, served, f.ID, len(f.Bands))
		}
		for j, b := range f.Bands {
			if served.Bands[j].Id != b.ID || served.Bands[j].Points != b.Points {
				t.Errorf("%s band %d served as %+v, want %+v", f.ID, j, served.Bands[j], b)
			}
		}
	}
}
