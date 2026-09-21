// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A litigation hold placed through the product, over a real database.
//
// The read side of the hold has always been built: every retention selector
// carries `NOT legal_hold` and the Art. 17 cascade refuses a held record. What
// never existed was a writer, so the column was false in every row of every
// installation and the guard could not be reached from outside the database.
//
// Driven over HTTP end to end, because the part that is new is the dispatch
// from an entity type to the module that owns its table — and a test calling a
// store directly would skip exactly that.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// placeHold asks the product to hold one record, returning the HTTP status so
// a caller can assert a refusal as readily as a success.
func placeHold(t *testing.T, e *apptest.AppEnv, entityType, id, reason string) int {
	t.Helper()
	return e.Call(t, "POST", "/v1/retention/legal-holds/"+entityType+"/"+id+"/place",
		AnyMap{"reason": reason}, nil, nil)
}

// liftHold is the other direction, same shape.
func liftHold(t *testing.T, e *apptest.AppEnv, entityType, id, reason string) int {
	t.Helper()
	return e.Call(t, "POST", "/v1/retention/legal-holds/"+entityType+"/"+id+"/lift",
		AnyMap{"reason": reason}, nil, nil)
}

// holdableRecords creates one of each of the five holdable types, so the
// dispatch can be asked about all of them.
func holdableRecords(t *testing.T, e *apptest.AppEnv) []struct{ entityType, table, id string } {
	t.Helper()
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Acme GmbH", "source": "manual"})
	pipeline, stage, _ := companyRollupOpenStage(t, e)
	return []struct{ entityType, table, id string }{
		{"contact", "contact", createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Dana Meyer", "source": "manual"})},
		{"company", "company", company},
		{"deal", "deal", createdID(t, e, "/v1/deals", AnyMap{
			"name": "Disputed renewal", "pipeline_id": pipeline, "stage_id": stage,
			"company_id": company, "source": "manual",
		})},
		{"lead", "lead", createdID(t, e, "/v1/leads", AnyMap{"full_name": "Sam Okafor", "source": "manual"})},
		{"project", "project", createdID(t, e, "/v1/projects", AnyMap{
			"name": "Disputed rollout", "company_id": company, "source": "manual",
		})},
	}
}

// heldOnTheRecord reads the flag back off the record's own GET, which is what
// a screen offering an erasure would consult.
func heldOnTheRecord(t *testing.T, e *apptest.AppEnv, collection, id string) bool {
	t.Helper()
	var record struct {
		LegalHold *bool `json:"legal_hold"`
	}
	if status := e.Call(t, "GET", "/v1/"+collection+"/"+id, nil, nil, &record); status != http.StatusOK {
		t.Fatalf("GET /v1/%s/%s = %d", collection, id, status)
	}
	if record.LegalHold == nil {
		t.Fatalf("GET /v1/%s/%s carried no legal_hold at all — a screen cannot tell held from unheld", collection, id)
	}
	return *record.LegalHold
}

// Every one of the five holdable records, through the one door.
//
// All five in one test because the dispatch IS the subject: four passing
// proves nothing about the fifth, and the defect this catches is an entity
// type the switch forgot — which looks like a refusal on exactly one type and
// like nothing at all on the others.
func TestEveryHoldableRecordCanBeHeldAndReleasedThroughTheProduct(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	collections := map[string]string{
		"contact": "contacts", "company": "companies", "deal": "deals",
		"lead": "leads", "project": "projects",
	}
	for _, k := range holdableRecords(t, e) {
		t.Run(k.entityType, func(t *testing.T) {
			collection := collections[k.entityType]
			if heldOnTheRecord(t, e, collection, k.id) {
				t.Fatalf("a %s was born held; the placement below would prove nothing", k.entityType)
			}
			if status := placeHold(t, e, k.entityType, k.id, "Anwaltsschreiben 2026-14"); status != http.StatusNoContent {
				t.Fatalf("placing a hold on a %s = %d, want 204", k.entityType, status)
			}
			if !heldOnTheRecord(t, e, collection, k.id) {
				t.Errorf("a %s reported a placed hold and its own read still says unheld", k.entityType)
			}
			// Placing twice is a conflict, not a second silent write: another
			// audit row would claim a change that did not happen.
			if status := placeHold(t, e, k.entityType, k.id, "again"); status != http.StatusConflict {
				t.Errorf("placing a hold twice on a %s = %d, want 409", k.entityType, status)
			}
			if status := liftHold(t, e, k.entityType, k.id, "Verfahren eingestellt"); status != http.StatusNoContent {
				t.Fatalf("lifting the hold on a %s = %d, want 204", k.entityType, status)
			}
			if heldOnTheRecord(t, e, collection, k.id) {
				t.Errorf("a %s reported a lifted hold and its own read still says held", k.entityType)
			}
			if status := liftHold(t, e, k.entityType, k.id, "again"); status != http.StatusConflict {
				t.Errorf("lifting an unheld %s = %d, want 409", k.entityType, status)
			}
		})
	}
}

// A hold with no stated reason is refused before anything is written. The
// reason has nowhere to live but the audit row, so an unstated one would leave
// the product holding a record with no recorded grounds at all.
func TestAHoldWithNoStatedReasonIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Dana Meyer", "source": "manual"})
	if status := e.Call(t, "POST", "/v1/retention/legal-holds/contact/"+contact+"/place",
		AnyMap{"reason": "   "}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("placing a hold with a blank reason = %d, want 422", status)
	}
	if heldOnTheRecord(t, e, "contacts", contact) {
		t.Error("the refused hold was written anyway")
	}
}

// What the controller's review surface answers, over records held through the
// product rather than through SQL.
func TestTheHoldListNamesWhatIsHeldAndNothingElse(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	held := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Dana Meyer", "source": "manual"})
	free := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Sam Okafor", "source": "manual"})
	if status := placeHold(t, e, "contact", held, "Anwaltsschreiben 2026-14"); status != http.StatusNoContent {
		t.Fatalf("placing the hold = %d, want 204", status)
	}

	var page struct {
		Data []struct {
			EntityType string `json:"entity_type"`
			RecordID   string `json:"record_id"`
			Label      string `json:"label"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/retention/legal-holds", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("listing holds = %d, want 200", status)
	}
	seen := map[string]string{}
	for _, row := range page.Data {
		seen[row.RecordID] = row.EntityType + ":" + row.Label
	}
	if got := seen[held]; got != "contact:Dana Meyer" {
		t.Errorf("the held contact reads as %q, want \"contact:Dana Meyer\"", got)
	}
	if _, listed := seen[free]; listed {
		t.Error("an unheld contact reached the hold list, which would make the list useless for reviewing a hold")
	}
}

// The list pages, and the second page continues the first rather than
// repeating it.
//
// `limit=1` rather than fifty rows: the keyset is what is under test, not the
// default page size, and a fixture sized to the default would be fifty holds
// to prove one cursor works.
func TestTheHoldListPagesWithoutRepeatingOrSkippingARecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	held := map[string]bool{}
	for _, name := range []string{"Dana Meyer", "Sam Okafor", "Mira Voss"} {
		id := createdID(t, e, "/v1/contacts", AnyMap{"full_name": name, "source": "manual"})
		if status := placeHold(t, e, "contact", id, "Anwaltsschreiben 2026-14"); status != http.StatusNoContent {
			t.Fatalf("placing a hold on %s = %d, want 204", name, status)
		}
		held[id] = true
	}

	seen := map[string]int{}
	cursor := ""
	for page := 0; page < len(held)+1; page++ {
		path := "/v1/retention/legal-holds?limit=1"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		var got struct {
			Data []struct {
				RecordID string `json:"record_id"`
			} `json:"data"`
			Page struct {
				NextCursor *string `json:"next_cursor"`
				HasMore    bool    `json:"has_more"`
			} `json:"page"`
		}
		if status := e.Call(t, "GET", path, nil, nil, &got); status != http.StatusOK {
			t.Fatalf("page %d = %d, want 200", page, status)
		}
		if len(got.Data) != 1 {
			t.Fatalf("page %d carried %d rows, want 1 — the limit was not honoured", page, len(got.Data))
		}
		seen[got.Data[0].RecordID]++
		if !got.Page.HasMore {
			break
		}
		if got.Page.NextCursor == nil {
			t.Fatal("a page says more follows and hands back no cursor, so a reader cannot ask for it")
		}
		cursor = *got.Page.NextCursor
	}

	// Every held record exactly once. Counting rather than comparing lengths:
	// a keyset that repeated a row and skipped another would hand back the
	// right NUMBER of rows and the wrong ones.
	for id := range held {
		if seen[id] != 1 {
			t.Errorf("held record %s appeared %d time(s) across the pages, want exactly 1", id, seen[id])
		}
	}
	for id := range seen {
		if !held[id] {
			t.Errorf("the pages carried %s, which is not held", id)
		}
	}
}
