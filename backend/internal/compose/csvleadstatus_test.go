// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestALeadStatusCellIsReadInAnyCaseAndLandsOnTheOpenLadder(t *testing.T) {
	for _, open := range contacts.OpenLeadStatuses() {
		typed, problem := checkLeadStatus(map[string]string{"status": " " + strings.ToUpper(open) + " "}, nil)
		if problem != nil || typed["status"] != open {
			t.Errorf("%s: typed %v, problem %v, want the status typed as written in lower case", open, typed, problem)
		}
	}
	in := leadCreateFrom(map[string]string{"status": "Engaged"}, map[string]any{"status": "engaged"}, "import:csv", "L-1", "src")
	if in.Status != "engaged" {
		t.Errorf("created as %q, want engaged", in.Status)
	}
	if got := leadCreateFrom(map[string]string{}, nil, "import:csv", "L-2", "src").Status; got != "new" {
		t.Errorf("a file with no status column created %q, want new", got)
	}
}

func TestALeadStatusTheStoreOnlyReachesByAnActionIsRefusedWithTheAllowedList(t *testing.T) {
	allowed := strings.Join(contacts.OpenLeadStatuses(), ", ")
	cases := map[string]migration.ReasonCode{
		"promoted":     migration.ReasonLeadStatusNotImportable,
		"Disqualified": migration.ReasonLeadStatusNotImportable,
		"hot":          migration.ReasonLeadStatusUnknown,
	}
	for cell, want := range cases {
		_, problem := checkLeadStatus(map[string]string{"status": cell}, nil)
		if problem == nil || problem.Why.Code != want || problem.Why.Params["allowed"] != allowed {
			t.Errorf("%q: got %+v, want %s listing %q", cell, problem, want, allowed)
		}
	}
	if _, problem := checkLeadStatus(map[string]string{"status": ""}, nil); problem != nil {
		t.Errorf("an empty cell is refused: %+v", problem)
	}
}

func TestTheStatusTheFileCarriesIsTheStoresOwnOpenVocabulary(t *testing.T) {
	// The store refuses the terminal pair on create and edit. The file's list
	// is the same list, so a status added to the ladder reaches the file.
	for _, status := range terminalLeadStatuses {
		if slices.Contains(contacts.OpenLeadStatuses(), status) {
			t.Errorf("%s is on the open ladder, so the file's terminal list is stale", status)
		}
	}
	if len(contacts.OpenLeadStatuses()) == 0 {
		t.Fatal("the store answers no open statuses")
	}
}

func TestAnEmptyStatusBoxIsNeverAnUpdate(t *testing.T) {
	stored := []byte(`{"status":"contacted"}`)
	changed, err := changedFields(stored, map[string]string{"status": ""}, nil)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v, err %v, want nothing", changed, err)
	}
	changed, err = changedFields(stored, map[string]string{"status": "Contacted"}, map[string]any{"status": "contacted"})
	if err != nil || len(changed) != 0 {
		t.Fatalf("an unedited status read as changed: %v, err %v", changed, err)
	}
	changed, _ = changedFields(stored, map[string]string{"status": "engaged"}, map[string]any{"status": "engaged"})
	if _, ok := changed["status"]; !ok {
		t.Fatal("a different status is not an update")
	}
}

// leadRoundTrip is the lead arm of TestEveryImportTargetRoundTripsThroughCreateAndUpdate:
// which targets reach the create input and which reach the patch.
func leadRoundTrip(fields map[string]string, typed map[string]any) (created, patched map[string]bool) {
	// The status cell is typed by the row's check before it is written.
	typed["status"] = "engaged"
	in := leadCreateFrom(fields, typed, "import:csv", "ext-1", "src")
	up := leadUpdateFrom(fields, typed)
	created = map[string]bool{
		"full_name":     in.FullName != nil,
		"email":         in.Email != nil,
		"phone":         in.Phone != nil,
		"title":         in.Title != nil,
		"company_name":  in.CompanyName != nil,
		"linkedin_url":  in.LinkedInURL != nil,
		"origin":        in.OriginHint != "",
		"origin_detail": in.OriginDetail != nil,
		"author":        in.Author.AuthorName != nil,
		"owner_email":   in.OwnerID != nil,
		"cf_tier":       in.CustomFields["cf_tier"] != nil,
		"status":        in.Status == "engaged",
	}
	patched = map[string]bool{
		"status":        up.Status != nil,
		"full_name":     up.FullName != nil,
		"email":         up.Email != nil,
		"phone":         up.Phone != nil,
		"title":         up.Title != nil,
		"company_name":  up.CompanyName != nil,
		"linkedin_url":  up.LinkedInURL != nil,
		"origin":        up.OriginHint != "",
		"origin_detail": up.OriginDetail != nil,
		"owner_email":   up.OwnerID != nil,
		"cf_tier":       up.CustomFields["cf_tier"] != nil,
	}
	return created, patched
}
