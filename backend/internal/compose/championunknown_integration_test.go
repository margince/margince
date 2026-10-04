// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the at-risk lane says about a deal's champion, against a real database
// and through the writers that land the records: the importer's own HTTP door
// for imported deals, the deals store for a deal a rep typed, and the contacts
// store for committee seats.

import (
	"encoding/json"
	"net/http"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// championStage is where every deal in these tests is born.
type championStage struct {
	pipeline ids.PipelineID
	stage    ids.StageID
}

func newChampionStage(t *testing.T, e *integration.Env) championStage {
	t.Helper()
	pipeline, stage, _ := integration.DealFixture(t, e)
	return championStage{pipeline: pipeline, stage: stage}
}

// importedQuietDeal lands a deal through POST /v1/deals as a declared importer,
// then lets it go quiet.
func importedQuietDeal(t *testing.T, e *integration.Env, at championStage, name string, company ids.UUID) ids.UUID {
	t.Helper()
	system, companyID := "mirror:hubspot", openapi_types.UUID(company)
	raw, err := json.Marshal(crmcontracts.CreateDealRequest{
		Name: name, PipelineId: openapi_types.UUID(at.pipeline.UUID), StageId: openapi_types.UUID(at.stage.UUID),
		Source: "import", SourceSystem: &system, CompanyId: &companyID,
	})
	if err != nil {
		t.Fatalf("encoding the deal: %v", err)
	}
	status, id := serveCreate(e.As(e.AdminUser, nil, importerPerms()), t, "/v1/deals", raw,
		func(w http.ResponseWriter, r *http.Request) {
			deals.NewHandlers(e.DB(), DealsInstallation()).CreateDeal(w, r, crmcontracts.CreateDealParams{})
		})
	if status != http.StatusCreated {
		t.Fatalf("importing %q answered %d, want 201", name, status)
	}
	deal, err := ids.Parse(id)
	if err != nil {
		t.Fatalf("reading the created id %q: %v", id, err)
	}
	idleFor(t, e, deal, 21)
	return deal
}

// typedQuietDeal is a deal a rep entered here, gone quiet.
func typedQuietDeal(t *testing.T, e *integration.Env, at championStage, name string, company ids.UUID) ids.UUID {
	t.Helper()
	companyID := ids.From[ids.CompanyKind](company)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: at.pipeline, StageID: at.stage, CompanyID: &companyID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	idleFor(t, e, ids.UUID(deal.Id), 21)
	return ids.UUID(deal.Id)
}

// seatOn puts a contact on a deal's committee through the contacts store.
func seatOn(t *testing.T, e *integration.Env, deal ids.UUID, name string, role *string) {
	t.Helper()
	contact := ids.From[ids.ContactKind](e.SeedContact(t, name, &e.AdminUser))
	dealID := ids.From[ids.DealKind](deal)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contact, DealID: &dealID, Role: role, Source: "manual",
	}); err != nil {
		t.Fatalf("seating %s: %v", name, err)
	}
}

// atRiskByDeal runs the morning queue's at-risk lane as the admin.
func atRiskByDeal(t *testing.T, e *integration.Env) map[ids.UUID]attention.RiskyDeal {
	t.Helper()
	lane := attentionAtRisk{
		lister: quietDealScanWithClock(e.Pool, deals.QuietThresholdDays, clockNow),
		pool:   e.Pool,
		now:    clockNow,
	}
	rows, _, err := lane.Quiet(e.Admin())
	if err != nil {
		t.Fatalf("reading the at-risk lane: %v", err)
	}
	byDeal := make(map[ids.UUID]attention.RiskyDeal, len(rows))
	for _, row := range rows {
		byDeal[row.DealID] = row
	}
	return byDeal
}

// "Champion unknown" is said only of an imported deal, where the source may not
// have recorded a champion. A deal created here with nobody recorded says
// nothing about its champion, and one with contacts recorded and none of them
// champion still says nobody is carrying it.
func TestChampionUnknownIsSaidOnlyOfAnImportedDeal(t *testing.T) {
	e := integration.Setup(t)
	at := newChampionStage(t, e)
	company := e.SeedCompany(t, "Turbinenbau", nil)
	decider, champion := "decision_maker", "champion"

	recorded := typedQuietDeal(t, e, at, "Recorded committee", company)
	seatOn(t, e, recorded, "Dana Decider", &decider)
	seatless := typedQuietDeal(t, e, at, "Seatless deal", company)
	importedSeatless := importedQuietDeal(t, e, at, "Imported, nobody recorded", company)
	importedCommittee := importedQuietDeal(t, e, at, "Imported committee", company)
	seatOn(t, e, importedCommittee, "Ines Imported", &decider)
	named := importedQuietDeal(t, e, at, "Imported, champion named here", company)
	seatOn(t, e, named, "Nora Named", &champion)

	rows := atRiskByDeal(t, e)
	for _, tc := range []struct {
		name                string
		deal                ids.UUID
		noChampion, unknown bool
	}{
		{"a committee recorded here with no champion", recorded, true, false},
		{"a deal created here with nobody recorded", seatless, false, false},
		{"an imported deal with nobody recorded", importedSeatless, false, true},
		{"an imported committee naming no champion", importedCommittee, false, true},
		{"an imported deal whose named champion is quiet", named, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, found := rows[tc.deal]
			if !found {
				t.Fatal("the deal is not on the at-risk lane")
			}
			if got := row.NoChampion != nil && *row.NoChampion; got != tc.noChampion {
				t.Errorf("no champion = %v, want %v", got, tc.noChampion)
			}
			if row.ChampionUnknown != tc.unknown {
				t.Errorf("champion unknown = %v, want %v", row.ChampionUnknown, tc.unknown)
			}
		})
	}
}
