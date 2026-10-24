// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type importedAddressDTO struct {
	Line1      *string `json:"line1"`
	City       *string `json:"city"`
	PostalCode *string `json:"postal_code"`
	Country    *string `json:"country"`
}

func importedAddressOf(t *testing.T, e *apptest.AppEnv, name string) importedAddressDTO {
	t.Helper()
	var companies struct {
		Data []struct {
			DisplayName string              `json:"display_name"`
			Address     *importedAddressDTO `json:"address"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/companies?limit=100", nil, nil, &companies); status != http.StatusOK {
		t.Fatalf("listing → %d, want 200", status)
	}
	for _, o := range companies.Data {
		if o.DisplayName == name && o.Address != nil {
			return *o.Address
		}
	}
	t.Fatalf("no company %q with an address", name)
	return importedAddressDTO{}
}

func partText(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestCSVImportSplitsAWholeAddressMappedToLineOne(t *testing.T) {
	e := setupImportApp(t)
	const file = "Company,Address\n" +
		`Splitwerk GmbH,"Example Street 4, 12345 Example City, Germany"` + "\n" +
		"Loosewerk GmbH,Somewhere near the river\n"
	profile, _ := uploadCSV(t, e, "company", file)
	run, _ := createRunWithMapping(t, e, "company", profile.SourceRef,
		map[string]string{"Company": "display_name", "Address": "address.line1"})

	report := readImportReport(t, e, run.ID)
	if issue := issueOf(report, "invalid_value", "Somewhere near the river"); issue == nil || issue.Column != "Address" {
		t.Errorf("issues = %+v, want the unsplit address warned about on its column", report.Issues)
	}
	warned := 0
	for _, issue := range report.Issues {
		if issue.Column == "Address" {
			warned++
		}
	}
	if warned != 1 {
		t.Errorf("issues = %+v, want one warning on the Address column, none for the split row", report.Issues)
	}
	if status := importCall(t, e, run.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	got := importedAddressOf(t, e, "Splitwerk GmbH")
	if partText(got.Line1) != "Example Street 4" || partText(got.PostalCode) != "12345" || partText(got.City) != "Example City" || partText(got.Country) != "DE" {
		t.Errorf("address = %s / %s / %s / %s, want the four parts", partText(got.Line1), partText(got.PostalCode), partText(got.City), partText(got.Country))
	}
	if loose := importedAddressOf(t, e, "Loosewerk GmbH"); partText(loose.Line1) != "Somewhere near the river" || loose.City != nil {
		t.Errorf("unsplit address = %s / %s, want the cell kept whole in line 1", partText(loose.Line1), partText(loose.City))
	}
}

func TestCSVImportKeepsLineOneAsWrittenWhenTheCityIsMapped(t *testing.T) {
	e := setupImportApp(t)
	const file = "Company,Street,City\n" + `Mapwerk GmbH,"Example Street 4, 12345 Example City",Example City` + "\n"
	profile, _ := uploadCSV(t, e, "company", file)
	run, _ := createRunWithMapping(t, e, "company", profile.SourceRef,
		map[string]string{"Company": "display_name", "Street": "address.line1", "City": "address.city"})
	if status := importCall(t, e, run.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if got := importedAddressOf(t, e, "Mapwerk GmbH"); partText(got.Line1) != "Example Street 4, 12345 Example City" || got.PostalCode != nil {
		t.Errorf("address = %s / %s, want line 1 as the file wrote it", partText(got.Line1), partText(got.PostalCode))
	}
}
