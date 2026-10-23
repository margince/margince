// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Relationship edges + the partner extension over HTTP: endpoint
// visibility gates reads and writes (an edge never out-sees its ends),
// one current-primary employer per contact, optimistic concurrency on
// update, and partner promotion flipping the company's classification.

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

type relEnv struct {
	*apptest.AppEnv
	contactID string
	companyID string
}

func setupRelationships(t *testing.T) *relEnv {
	t.Helper()
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Rel E2E", "rel@fable.test", "Admin")
	var contact, company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Edge Contact"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": "Edge Company"}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}
	return &relEnv{AppEnv: e, contactID: contact.ID, companyID: company.ID}
}

func TestRelationshipLifecycle(t *testing.T) {
	e := setupRelationships(t)

	var first struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"role": "cto", "is_current_primary": true, "source": "manual",
	}, nil, &first); status != http.StatusCreated {
		t.Fatalf("create employment → %d", status)
	}

	// A second primary employer demotes the first inside one tx.
	var company2 struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": "Second Company"}, nil, &company2); status != http.StatusCreated {
		t.Fatalf("create company2 → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": company2.ID,
		"is_current_primary": true, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("second employment → %d", status)
	}
	var listed struct {
		Data []struct {
			ID               string `json:"id"`
			IsCurrentPrimary bool   `json:"is_current_primary"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/relationships?contact_id="+e.contactID+"&kind=employment", nil, nil, &listed); status != http.StatusOK || len(listed.Data) != 2 {
		t.Fatalf("list → %d %+v", status, listed)
	}
	primaries := 0
	for _, rel := range listed.Data {
		if rel.IsCurrentPrimary {
			primaries++
		}
	}
	if primaries != 1 {
		t.Fatalf("%d current-primary employers, the invariant is ≤1", primaries)
	}

	// Optimistic concurrency: a stale If-Match answers version_skew.
	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "PATCH", "/v1/relationships/"+first.ID, AnyMap{"role": "ceo"},
		map[string]string{"If-Match": "999"}, &problem)
	if status != http.StatusConflict || problem.Code != "version_skew" {
		t.Fatalf("stale If-Match → %d %q, want 409 version_skew", status, problem.Code)
	}
	if status := e.Call(t, "PATCH", "/v1/relationships/"+first.ID, AnyMap{"role": "ceo"}, nil, nil); status != http.StatusOK {
		t.Fatalf("update → %d", status)
	}

	if status := e.Call(t, "DELETE", "/v1/relationships/"+first.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive → %d", status)
	}

	// A malformed endpoint shape is a 422, not a DB error.
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "source": "manual",
	}, nil, nil); status != 422 {
		t.Fatalf("shape-violating edge → %d, want 422", status)
	}
	// An invisible endpoint reads as absent (H1), and the refusal names it.
	var refused problemBody
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": "00000000-0000-7000-8000-00000000dead",
		"company_id": e.companyID, "source": "manual",
	}, nil, &refused); status != http.StatusUnprocessableEntity || !namesFieldWithCode(refused, "contact_id", "reference_not_found") {
		t.Fatalf("invisible endpoint → %d %+v, want 422 reference_not_found on contact_id", status, refused.Details.Errors)
	}
}

// An agent archives an edge on its own passport, and the edge is gone.
//
// It used to stage: a 403 naming an approval, the edge parked until a human
// clicked. What changed is the tier, not the gate — a passport carries the
// granting human's seat and row scope, and `write` is a cap that human lent, so
// asking them to confirm an archive they could perform themselves in the web app
// bought nothing (ADR-0055).
//
// Under a tier floor, automatic work with no passport is refused the archive
// instead: TestAFlooredEdgeArchiveIsRefusedToAutomaticWork below.
func TestAnAgentArchivesAnEdgeOnItsOwnPassport(t *testing.T) {
	e := setupRelationships(t)

	var edge struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"role": "cto", "source": "manual",
	}, nil, &edge); status != http.StatusCreated {
		t.Fatalf("create employment → %d", status)
	}

	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "edge agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	if status := e.Call(t, "DELETE", "/v1/relationships/"+edge.ID, nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("agent archive → %d, want 200", status)
	}

	var listed struct {
		Data []struct {
			ID         string  `json:"id"`
			ArchivedAt *string `json:"archived_at"`
		} `json:"data"`
	}
	if got := e.Call(t, "GET", "/v1/relationships?contact_id="+e.contactID+"&kind=employment", nil, nil, &listed); got != http.StatusOK {
		t.Fatalf("list after the archive → %d", got)
	}
	for _, rel := range listed.Data {
		if rel.ID == edge.ID && rel.ArchivedAt == nil {
			t.Error("the edge is still live after the agent archived it")
		}
	}
}

// Under a tier floor, automatic work with no passport may not stage the archive
// the floor asks for. It is refused and the edge stays live.
func TestAFlooredEdgeArchiveIsRefusedToAutomaticWork(t *testing.T) {
	e := setupRelationships(t)

	var edge struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"role": "cto", "source": "manual",
	}, nil, &edge); status != http.StatusCreated {
		t.Fatalf("create employment → %d", status)
	}
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "edge agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}

	_, err := flooredArchiveInvoker(t, e.AppEnv, minted.Token)(
		`{"record_type":"relationship","id":"` + edge.ID + `"}`,
	)
	if !isPassportlessStagingRefusal(err) {
		t.Fatalf("a floored edge archive from automatic work answered %v, want the refusal that it cannot stage", err)
	}
	var archived bool
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT archived_at IS NOT NULL FROM relationship WHERE id = $1`, edge.ID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived {
		t.Error("the edge was archived by a refused call")
	}
}

// flooredArchiveInvoker calls archive_record through a registry with a tier
// floor on it, which is the composition an installation gets when it declares
// the verb confirm-first. It acts as automatic work: the token's colleague with
// no passport behind it. An agent on a passport archives as that colleague could.
func flooredArchiveInvoker(t *testing.T, e *apptest.AppEnv, agentToken string) func(string) (string, error) {
	t.Helper()
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	// After construction: the composition appends its own contract floor to any
	// option it is passed, so one supplied there would be overwritten.
	agents.WithTierFloor(func(tool, _ string) (mcp.RiskTier, bool) {
		if tool != "archive_record" {
			return mcp.TierAutoExecute, false
		}
		return mcp.TierConfirmationRequired, true
	})(registry)

	authSvc := identity.NewService(e.Pool)
	return func(args string) (string, error) {
		wsID, err := authSvc.InstallationWorkspace(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ctx := principal.WithWorkspaceID(context.Background(), wsID.UUID)
		agent, err := authSvc.AuthenticateAgent(ctx, agentToken)
		if err != nil {
			t.Fatal(err)
		}
		automatic := agent.Principal()
		automatic.PassportID, automatic.ConnectionID = ids.UUID{}, ids.UUID{}
		ctx = principal.WithCorrelationID(principal.WithActor(ctx, automatic), ids.NewV7())
		out, invokeErr := registry.Invoke(ctx, "archive_record", json.RawMessage(args))
		return string(out), invokeErr
	}
}

func TestPartnerPromotionLifecycle(t *testing.T) {
	e := setupRelationships(t)

	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/partner", nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("non-partner company → %d, want 404", status)
	}
	var partner struct {
		CertStatus string `json:"cert_status"`
	}
	if status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", AnyMap{
		"partner_role": "consulting", "cert_status": "certified",
		"gate_metrics": AnyMap{"certified_staff": 3, "retention_rate": 90},
	}, nil, &partner); status != http.StatusOK || partner.CertStatus != "certified" {
		t.Fatalf("upsert partner → %d %+v", status, partner)
	}
	// Promotion writes the partner relationship type — the half of the
	// invariant that lives on the company (ADR-0079 amending ADR-0032).
	var company struct {
		RelationshipTypes []string `json:"relationship_types"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID, nil, nil, &company); status != http.StatusOK {
		t.Fatalf("company after promotion → %d", status)
	}
	if !slices.Contains(company.RelationshipTypes, "partner") {
		t.Fatalf("company after promotion carries %v, want partner among them", company.RelationshipTypes)
	}
	var partners struct {
		Data []struct {
			CompanyID string `json:"company_id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/partners?cert_status=certified", nil, nil, &partners); status != http.StatusOK || len(partners.Data) != 1 {
		t.Fatalf("list partners → %d %+v", status, partners)
	}
	if status := e.Call(t, "GET", "/v1/partners?cert_status=suspended", nil, nil, &partners); status != http.StatusOK {
		t.Fatalf("filtered list → %d", status)
	}
}
