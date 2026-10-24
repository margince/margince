// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
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

func TestCSVUpdateWithASplitLineOneClearsTheStaleParts(t *testing.T) {
	e := setupImportApp(t)
	const full = "Company,Street,Suite,Region,Postal,City,Country\n" +
		"Movewerk GmbH,Old Road 2,Old Suite,CA,94043,Mountain View,US\n"
	p1, _ := uploadCSV(t, e, "company", full)
	r1, _ := createRunWithMapping(t, e, "company", p1.SourceRef, map[string]string{
		"Company": "display_name", "Street": "address.line1", "Suite": "address.line2", "Region": "address.region",
		"Postal": "address.postal_code", "City": "address.city", "Country": "address.country",
	})
	if status := importCall(t, e, r1.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve 1 → %d, want 202", status)
	}
	const moved = "Company,Address\n" + `Movewerk GmbH,"Neue Straße 1, 10115 Berlin, Germany"` + "\n"
	p2, _ := uploadCSV(t, e, "company", moved)
	r2, _ := createRunWithMapping(t, e, "company", p2.SourceRef, map[string]string{"Company": "display_name", "Address": "address.line1"})
	if status := importCall(t, e, r2.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve 2 → %d, want 202", status)
	}
	var companies struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Address     struct {
				Line1, Line2, Region, City, Country *string
				PostalCode                          *string `json:"postal_code"`
			} `json:"address"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/companies?limit=100", nil, nil, &companies); status != http.StatusOK {
		t.Fatalf("listing → %d, want 200", status)
	}
	found := false
	for _, o := range companies.Data {
		if o.DisplayName != "Movewerk GmbH" {
			continue
		}
		found = true
		a := o.Address
		got := []string{partText(a.Line1), partText(a.Line2), partText(a.Region), partText(a.PostalCode), partText(a.City), partText(a.Country)}
		want := []string{"Neue Straße 1", "<nil>", "<nil>", "10115", "Berlin", "DE"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("address = %q, want %q", got, want)
		}
	}
	if !found {
		t.Fatal("the company was not found")
	}
}
