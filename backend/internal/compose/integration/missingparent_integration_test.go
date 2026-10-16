// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A sub-resource read of a record that is not there says so. A 200 with an
// empty list reads as "nothing here yet" to a client holding a stale or
// mistyped id.
func TestSubResourceReadsOfAMissingParentAnswerNotFound(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	const missing = "00000000-0000-7000-8000-0000000000ee"

	for _, route := range []string{
		"/v1/contacts/" + missing + "/intro-requests",
		"/v1/projects/" + missing + "/health-assessments",
		"/v1/leads/" + missing + "/manual-signals",
		"/v1/deals/" + missing + "/outcome-reviews",
		"/v1/deals/" + missing + "/documents",
		"/v1/field-history?entity_type=deal&entity_id=" + missing,
		"/v1/field-history?entity_type=lead&entity_id=" + missing,
		"/v1/field-history?entity_type=project&entity_id=" + missing,
		"/v1/field-history?entity_type=contact&entity_id=" + missing,
		"/v1/records/deal/" + missing + "/history",
		"/v1/records/lead/" + missing + "/history",
		"/v1/companies/" + missing + "/documents",
		"/v1/attachments?entity_type=contact&entity_id=" + missing,
		"/v1/attachments?entity_type=company&entity_id=" + missing,
		"/v1/records/company/" + missing + "/assignments",
		"/v1/records/deal/" + missing + "/assignments",
		"/v1/records/project/" + missing + "/assignments",
		"/v1/scheduling/proposals?contact_id=" + missing,
	} {
		t.Run(route, func(t *testing.T) {
			if status := e.Call(t, "GET", route, nil, nil, nil); status != http.StatusNotFound {
				t.Errorf("GET %s → %d, want 404", route, status)
			}
		})
	}
}

// The agent surface reads the same library the tabs read, so a missing parent
// is refused there too, for every record type the tool names.
func TestListDocumentsOfAMissingRecordIsRefusedOnTheAgentDoor(t *testing.T) {
	d := newTwoDoors(t, "missingparent", "read")
	const missing = "00000000-0000-7000-8000-0000000000ee"

	for _, entityType := range []string{"company", "deal", "contact", "lead", "project"} {
		t.Run(entityType, func(t *testing.T) {
			d.mcp.CallRefused(t, "list_documents", map[string]any{"entity_type": entityType, "entity_id": missing})
		})
	}
}

func TestResearchSaveRefusesAMissingContactAndADroppedClaimsField(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	const missing = "00000000-0000-7000-8000-0000000000ee"
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Research Target"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}

	cases := []struct {
		name string
		id   string
		body AnyMap
		want int
	}{
		{"missing contact, empty claims", missing, AnyMap{"claims": []any{}}, http.StatusNotFound},
		{"missing contact, no body fields", missing, AnyMap{}, http.StatusUnprocessableEntity},
		{"known contact, empty claims saves nothing", contact.ID, AnyMap{"claims": []any{}}, http.StatusOK},
		{"known contact, no claims field", contact.ID, AnyMap{}, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if status := e.Call(t, "POST", "/v1/contacts/"+c.id+"/research/save", c.body, nil, nil); status != c.want {
				t.Errorf("research/save (%s) → %d, want %d", c.name, status, c.want)
			}
		})
	}
}
