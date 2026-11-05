// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CreateDealClosingThisWeek seeds one open deal that closes in three days, so
// its timing ranks it into the rep's brief.
func CreateDealClosingThisWeek(t *testing.T, e *AppEnv, stages SeededStages, name string) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deals", dealCreate{
		Name: name, PipelineID: stages.PipelineID, StageID: stages.Open,
		ExpectedCloseDate: clocktest.Now(t).UTC().AddDate(0, 0, 3).Format("2006-01-02"),
		Source:            provenance.RecordSourceManual,
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create deal %q status = %d", name, status)
	}
	return created.ID
}

type dealCreate struct {
	Name              string `json:"name"`
	PipelineID        string `json:"pipeline_id"`
	StageID           string `json:"stage_id"`
	ExpectedCloseDate string `json:"expected_close_date"`
	Source            string `json:"source"`
}

type companyCreate struct {
	Source      string          `json:"source"`
	DisplayName string          `json:"display_name"`
	Domains     []companyDomain `json:"domains,omitempty"`
}

type companyDomain struct {
	Domain    string `json:"domain"`
	IsPrimary bool   `json:"is_primary"`
}

type contactCreate struct {
	FullName string `json:"full_name"`
	Source   string `json:"source"`
}

type employmentCreate struct {
	Kind      string `json:"kind"`
	ContactID string `json:"contact_id"`
	CompanyID string `json:"company_id"`
	Source    string `json:"source"`
}

// BriefItem is one item of a seat's brief as the route answers it.
type BriefItem struct {
	ID           string   `json:"id"`
	DealID       string   `json:"deal_id"`
	Rank         int      `json:"rank"`
	EvidenceIds  []string `json:"evidence_ids"`
	State        string   `json:"state"`
	SnoozedUntil string   `json:"snoozed_until"`
}

// SeedScoutedNamed raises one deal suggestion: a company and its contact of
// the caller's naming, the buyer's request, and the deal scout's pass over it.
func SeedScoutedNamed(t *testing.T, e *AppEnv, companyName, contactName string) (companyID, suggestionID string) {
	t.Helper()
	var company, contact, source struct {
		ID ids.UUID `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", companyCreate{Source: provenance.RecordSourceManual, DisplayName: companyName}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company = %d %v", status, company)
	}
	companyID = company.ID.String()
	if status := e.Call(t, "POST", "/v1/contacts", contactCreate{FullName: contactName, Source: provenance.RecordSourceManual}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact = %d %v", status, contact)
	}
	contactID := contact.ID.String()
	if status := e.Call(t, "POST", "/v1/relationships", employmentCreate{
		Kind: "employment", ContactID: contactID, CompanyID: companyID, Source: provenance.RecordSourceManual,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create employment = %d", status)
	}
	now := clocktest.Now(t)
	if status := e.Call(t, "POST", "/v1/activities", map[string]any{
		"kind": "email", "subject": "Platform request", "direction": "inbound",
		"body":        "We need a platform migration. Please send a proposal; I will send our requirements.",
		"occurred_at": now.Add(-72 * time.Hour).UTC().Format(time.RFC3339),
		"links":       []map[string]any{{"entity_type": "contact", "entity_id": contactID}},
	}, nil, &source); status != http.StatusCreated {
		t.Fatalf("log buyer request = %d %v", status, source)
	}

	ws := InstallationWorkspaceUUID(context.Background(), t, e.Pool)
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), "agent:deal-scout")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		for _, kind := range []string{"new_opportunity", "commitment_made"} {
			if _, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
				Kind: kind, CompanyID: company.ID, Summary: "Platform migration requested.", Severity: "info",
				Fingerprint: kind + ":" + source.ID.String(),
				Evidence: []signals.DerivedEvidence{{
					Snippet: "Platform migration requested.", ActivityID: source.ID,
					BuyerContactID: &contact.ID, CommercialRole: "buyer",
				}},
			}, now); err != nil {
				return err
			}
		}
		_, err := compose.RunDealScout(ctx, tx, now)
		return err
	}); err != nil {
		t.Fatalf("the scout pass: %v", err)
	}
	var list struct {
		Data []map[string]any `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-suggestions?company_id="+companyID, nil, nil, &list); status != http.StatusOK || len(list.Data) != 1 {
		t.Fatalf("list suggestions = %d with %d rows, want one", status, len(list.Data))
	}
	suggestionID, _ = list.Data[0]["id"].(string)
	return companyID, suggestionID
}

// SeedCompanyPair creates an incumbent company and a same-stem near-duplicate
// through the real create endpoint; the create records the queue pair.
func SeedCompanyPair(t *testing.T, e *AppEnv, stem, incumbentDomain, dupDomain string) (incumbentID string) {
	t.Helper()
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", companyCreate{
		Source: provenance.RecordSourceManual, DisplayName: stem + " GmbH",
		Domains: []companyDomain{{Domain: incumbentDomain, IsPrimary: true}},
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("incumbent create → %d, want 201", status)
	}
	if status := e.Call(t, "POST", "/v1/companies", companyCreate{
		Source: provenance.RecordSourceManual, DisplayName: stem + " Inc",
		Domains: []companyDomain{{Domain: dupDomain, IsPrimary: true}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("fuzzy create must still 201, got %d", status)
	}
	return company.ID
}

// SeedRoute gives the colleague seat a recorded edge to the contact, so a route
// to them exists. It goes through the real edge recompute, because a route the
// graph would not return proves nothing about the endpoint.
func SeedRoute(t *testing.T, e *AppEnv, seat, contact ids.UUID) {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, audience, source, captured_by)
		VALUES ($1, 'email', 'Re: Angebot', now() - interval '3 days', 'outbound',
		        'workspace', 'manual', 'human:x')`, id); err != nil {
		t.Fatalf("seeding the exchange: %v", err)
	}
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, id, contact); err != nil {
		t.Fatalf("linking the exchange: %v", err)
	}
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`,
		id, seat); err != nil {
		t.Fatalf("seeding our side: %v", err)
	}
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'to')`,
		id, contact); err != nil {
		t.Fatalf("seeding their side: %v", err)
	}
	ws := InstallationWorkspaceUUID(ctx, t, e.Pool)
	wsCtx := principal.WithWorkspaceID(ctx, ws)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, []ids.UUID{id})
	}); err != nil {
		t.Fatalf("folding the edge: %v", err)
	}
}

// LogoPNG is a small square PNG standing in for a normalized mark.
func LogoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return out.Bytes()
}
