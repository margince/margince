// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordexport

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func record(t *testing.T, doc string) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &out); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return out
}

func fromJSON(record map[string]json.RawMessage, target string) string {
	var text string
	if json.Unmarshal(record[target], &text) != nil {
		return string(record[target])
	}
	return text
}

func TestTheFileEndsWithTheIDAndLeavesOutWhatNoRecordCarries(t *testing.T) {
	got := ExportedTargets("deal", []string{"name", "contact_ref", "amount"})
	if want := []string{"name", "amount", "id"}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want %v: a stakeholder edge is no column of the deal", got, want)
	}
	if got := ExportedTargets("contact", []string{"author", "email"}); !slices.Equal(got, []string{"email", "id"}) {
		t.Errorf("targets = %v, want the author, which the import writes on create only, left out", got)
	}
}

func TestACSVStartsWithTheByteOrderMarkExcelNeeds(t *testing.T) {
	file, err := CSV([]string{"full_name"}, [][]string{{"Müller"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(file, ByteOrderMark) {
		t.Fatalf("file = %q, want the UTF-8 byte order mark first", file)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(file, ByteOrderMark))).ReadAll()
	if err != nil || len(records) != 2 || records[1][0] != "Müller" {
		t.Fatalf("records = %v, %v, want the header and Müller", records, err)
	}
}

func TestADealAmountIsWrittenInMajorUnitsOfItsCurrency(t *testing.T) {
	spec := Spec{Object: "deal", Stored: fromJSON}
	for _, tc := range []struct{ name, doc, want string }{
		{"two decimals", `{"amount_minor": 1200050, "currency": "EUR"}`, "12000.50"},
		{"no decimals", `{"amount_minor": 5000, "currency": "JPY"}`, "5000"},
		{"masked amount", `{"currency": "EUR"}`, ""},
		{"no currency", `{"amount_minor": 100}`, ""},
	} {
		if got := cell(spec, lookups{}, record(t, tc.doc), Amount); got != tc.want {
			t.Errorf("%s: amount = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWordsReplaceTheIDsARecordNames(t *testing.T) {
	owner, company := ids.NewV7(), ids.NewV7()
	names := lookups{
		owners:    map[ids.UUID]string{owner: "ada@example.com"},
		companies: map[ids.UUID]string{company: "Müller & Söhne"},
	}
	doc := `{"owner_id": "` + owner.String() + `", "company_id": "` + company.String() +
		`", "tags": [{"name": "vip"}, {"name": "nord"}], "employer": {"company_name": "Acme"}}`
	spec := Spec{Object: "contact", Stored: fromJSON}
	for target, want := range map[string]string{
		OwnerEmail: "ada@example.com", CompanyRef: "Müller & Söhne", Tags: "vip; nord", EmployerName: "Acme",
	} {
		if got := cell(spec, names, record(t, doc), target); got != want {
			t.Errorf("%s = %q, want %q", target, got, want)
		}
	}
}

func TestALeadKeepsItsStatusOnlyOnTheOpenLadder(t *testing.T) {
	spec := Spec{Object: "lead", Stored: fromJSON}
	if got := cell(spec, lookups{}, record(t, `{"status": "new"}`), LeadStatus); got != "new" {
		t.Errorf("open status = %q, want new", got)
	}
	if got := cell(spec, lookups{}, record(t, `{"status": "disqualified"}`), LeadStatus); got != "" {
		t.Errorf("closed status = %q, want empty: the import takes only the open ladder", got)
	}
}
