// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The importer's door, driven over HTTP the way a request reaches it.
//
// The unit tests hold the two mappers and auth.DeclaredImporter separately.
// What NONE of them can show is the wiring: that the handler asks the question
// at all, that it asks about the principal on the request rather than anything
// in the body, and that the value reaches the column. A handler that never
// called DeclaredImporter would pass every unit test in this change.
//
// package compose, not compose/integration, because attributionrepair's
// repair() helper drives the unexported attributionHandlers and the last case
// here needs it.
//
// Four cases, each a different caller against the same body:
//
//   - a declared importer lands an activity and a lead inside the namespace,
//     and the attribution repair then answers `applied`;
//   - that human's AGENT, carrying the identical grants, is refused 422 — the
//     HTTP-level proof that the type check is load-bearing;
//   - a human WITHOUT import_run:create is refused 422;
//   - the door does not open `email`, which is no writer's to claim.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// importerPerms is AdminPerms plus the one grant the door reads. The harness
// admin does NOT hold import_run, so this addition is what makes the caller an
// importer — and its absence is what the refusal cases rely on.
func importerPerms() principal.Permissions {
	perms := integration.AdminPerms
	objects := make(map[string]principal.ObjectGrant, len(perms.Objects)+1)
	for k, v := range perms.Objects {
		objects[k] = v
	}
	objects["import_run"] = principal.ObjectGrant{Create: true}
	perms.Objects = objects
	return perms
}

// postActivity drives POST /activities as `as` and answers the status plus the
// decoded body.
func postActivity(as context.Context, t *testing.T, e *integration.Env, req crmcontracts.CreateActivityRequest) (int, crmcontracts.Activity) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("encoding the activity: %v", err)
	}
	rec := httptest.NewRecorder()
	activities.NewHandlers(InstallationDB(e.Pool)).LogActivity(
		rec,
		httptest.NewRequest(http.MethodPost, "/v1/activities", bytes.NewReader(raw)).WithContext(as),
		crmcontracts.LogActivityParams{},
	)
	var out crmcontracts.Activity
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the activity: %v", err)
		}
	}
	return rec.Code, out
}

// postLead is postActivity's twin on POST /leads.
func postLead(as context.Context, t *testing.T, e *integration.Env, req crmcontracts.CreateLeadRequest) (int, crmcontracts.Lead) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("encoding the lead: %v", err)
	}
	rec := httptest.NewRecorder()
	contacts.NewHandlers(InstallationDB(e.Pool)).CreateLead(
		rec,
		httptest.NewRequest(http.MethodPost, "/v1/leads", bytes.NewReader(raw)).WithContext(as),
		crmcontracts.CreateLeadParams{},
	)
	var out crmcontracts.Lead
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the lead: %v", err)
		}
	}
	return rec.Code, out
}

func namespacedActivity() crmcontracts.CreateActivityRequest {
	system := "mirror:hubspot"
	return crmcontracts.CreateActivityRequest{
		Kind: "email", Subject: strPtrIT("Betreff"), SourceSystem: &system,
		SourceId: strPtrIT("emails:900"),
	}
}

func namespacedLead() crmcontracts.CreateLeadRequest {
	system := "mirror:hubspot"
	return crmcontracts.CreateLeadRequest{
		FullName: strPtrIT("Imported Lead"), SourceSystem: &system,
		SourceId: strPtrIT("leads:501"), Source: "hubspot_import",
	}
}

func strPtrIT(s string) *string { return &s }

func TestADeclaredImporterLandsRowsInsideItsNamespace(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.AdminUser, nil, importerPerms())

	status, activity := postActivity(importer, t, e, namespacedActivity())
	if status != http.StatusCreated {
		t.Fatalf("the activity answered %d, want 201 — the importer may stamp its own namespace", status)
	}
	if got := e.WsScalar(t, `SELECT source_system FROM activity WHERE id = $1`, ids.UUID(activity.Id)); got != "mirror:hubspot" {
		t.Errorf("activity.source_system = %q, want mirror:hubspot — half the replay key", got)
	}

	status, lead := postLead(importer, t, e, namespacedLead())
	if status != http.StatusCreated {
		t.Fatalf("the lead answered %d, want 201", status)
	}
	if got := e.WsScalar(t, `SELECT source_system FROM lead WHERE id = $1`, ids.UUID(lead.Id)); got != "mirror:hubspot" {
		t.Errorf("lead.source_system = %q, want mirror:hubspot", got)
	}

	// What the namespace is FOR: the row now ranks as captured history and the
	// repair can name its author.
	out := repair(t, e, recordHandlers(e), crmcontracts.SourceAttributionRequest{
		BatchRef: "importer-door", Rows: []crmcontracts.SourceAttributionRow{
			row(ids.UUID(activity.Id), "Mutaz Suleiman", 1),
		},
	})
	if out.Applied != 1 {
		t.Errorf("the repair answered %+v, want the imported activity attributed", out)
	}
}

// The case the unit tests cannot reach, and the reason DeclaredImporter checks
// the principal TYPE before the grant: an agent carries its granting human's
// whole Permissions, so this caller holds import_run:create and must still be
// refused.
func TestAnAgentCarryingTheImportGrantIsStillRefused(t *testing.T) {
	e := integration.Setup(t)
	agent := e.AgentFor(t, e.AdminUser, nil, importerPerms())

	if status, _ := postActivity(agent, t, e, namespacedActivity()); status != http.StatusUnprocessableEntity {
		t.Errorf("the activity answered %d, want 422 — an agent may not become the importer", status)
	}
	if status, _ := postLead(agent, t, e, namespacedLead()); status != http.StatusUnprocessableEntity {
		t.Errorf("the lead answered %d, want 422 — an agent may not become the importer", status)
	}
}

// The grant is what opens the door, so a human without it is an ordinary
// client. Without this case the door could be admitting every human.
func TestAHumanWithoutTheImportGrantIsRefused(t *testing.T) {
	e := integration.Setup(t)
	ordinary := e.As(e.AdminUser, nil, integration.AdminPerms)

	if status, _ := postActivity(ordinary, t, e, namespacedActivity()); status != http.StatusUnprocessableEntity {
		t.Errorf("the activity answered %d, want 422 — import_run:create is what opens this door", status)
	}
	if status, _ := postLead(ordinary, t, e, namespacedLead()); status != http.StatusUnprocessableEntity {
		t.Errorf("the lead answered %d, want 422", status)
	}
}

// `email` is the one identity every mail transport shares, and it sits outside
// the admission deliberately: an import that could claim it would collide with
// captured mail on the identity capture dedupes by.
func TestTheImporterDoorDoesNotOpenTheMailIdentity(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.AdminUser, nil, importerPerms())

	req := namespacedActivity()
	mail := connector.EmailSourceSystem
	req.SourceSystem = &mail

	if status, _ := postActivity(importer, t, e, req); status != http.StatusUnprocessableEntity {
		t.Errorf("the activity answered %d, want 422 — the mail identity is no writer's to claim", status)
	}
}
