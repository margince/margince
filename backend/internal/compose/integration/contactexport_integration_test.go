// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The whole-workspace bundle carries the same two columns in contact.csv and is
// a valid archive.
func TestExportBundleContactsCarryTheReachableEmail(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if status := e.Call(t, http.MethodPost, "/v1/auth/login", AnyMap{
		"email": "ada@example.com", "password": "correct-horse-battery",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("login → %d", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Wilma Weber",
		"emails": []AnyMap{{"email": "wilma@corp.example", "email_type": "work", "is_primary": true}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create contact → %d, want 201", status)
	}

	req, err := http.NewRequest(http.MethodGet, e.TS.URL+"/v1/exports/bundle", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("bundle request: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the bundle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bundle → %d, want 200", resp.StatusCode)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("the bundle is not a valid zip: %v", err)
	}
	records := readBundleCSV(t, archive, "contact.csv")
	at := slices.Index(records[0], "primary_email")
	if at < 0 || len(records) != 2 || records[1][at] != "wilma@corp.example" {
		t.Fatalf("contact.csv = %v, want a primary_email column holding the address", records)
	}
}

func readBundleCSV(t *testing.T, archive *zip.Reader, name string) [][]string {
	t.Helper()
	for _, f := range archive.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", name, err)
		}
		defer apptest.CloseBody(t, &http.Response{Body: rc})
		records, err := csv.NewReader(rc).ReadAll()
		if err != nil {
			t.Fatalf("%s is not valid CSV: %v", name, err)
		}
		return records
	}
	t.Fatalf("the bundle has no %s", name)
	return nil
}

// A contact export carries the address and number a mail merge needs: the
// primary ones, never a retired one, and empty for a contact that has none.
func TestContactExportCarriesTheReachableEmailAndPhone(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if status := e.Call(t, http.MethodPost, "/v1/auth/login", AnyMap{
		"email": "ada@example.com", "password": "correct-horse-battery",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("login → %d", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Wilma Weber",
		"emails": []AnyMap{
			{"email": "second@corp.example", "email_type": "work", "is_primary": false},
			{"email": "wilma@corp.example", "email_type": "work", "is_primary": true},
		},
		"phones": []AnyMap{{"phone": "+4930123456", "phone_type": "mobile", "is_primary": true}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create contact → %d, want 201", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "No Channels",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create bare contact → %d, want 201", status)
	}

	records := exportCSV(t, e, AnyMap{
		"object": "contact", "format": "csv",
		"filter": AnyMap{"field": "created_at", "op": "gte", "value": "2000-01-01"},
	})

	header := records[0]
	emailAt, phoneAt, nameAt := slices.Index(header, "primary_email"), slices.Index(header, "primary_phone"), slices.Index(header, "full_name")
	if emailAt < 0 || phoneAt < 0 || nameAt < 0 {
		t.Fatalf("header %v lacks primary_email, primary_phone or full_name", header)
	}
	byName := map[string][]string{}
	for _, row := range records[1:] {
		byName[row[nameAt]] = row
	}
	if got := byName["Wilma Weber"][emailAt]; got != "wilma@corp.example" {
		t.Errorf("primary_email = %q, want the primary address", got)
	}
	// A leading plus is a formula lead, so the cell carries the export's usual
	// quote guard like every other text cell.
	if got := byName["Wilma Weber"][phoneAt]; got != "'+4930123456" {
		t.Errorf("primary_phone = %q, want the primary number behind the formula guard", got)
	}
	if bare := byName["No Channels"]; bare[emailAt] != "" || bare[phoneAt] != "" {
		t.Errorf("a contact with no channels exported %q / %q, want both empty", bare[emailAt], bare[phoneAt])
	}
}
