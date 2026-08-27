// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The relationship-graph reads over HTTP (ADR-0078). These go through the real
// router, the real gates and the real payload shapes — the layer where a
// contract promise either holds or does not.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type networkColleagueDTO struct {
	UserID          string `json:"user_id"`
	DisplayName     string `json:"display_name"`
	Strength        *int   `json:"strength"`
	StrengthBucket  string `json:"strength_bucket"`
	Interactions90d int    `json:"interactions_90d"`
}

type personNetworkDTO struct {
	PersonID   string                `json:"person_id"`
	Colleagues []networkColleagueDTO `json:"colleagues"`
}

type coverageRiskDTO struct {
	Kind      string   `json:"kind"`
	Summary   string   `json:"summary"`
	PersonIDs []string `json:"person_ids"`
	UserIDs   []string `json:"user_ids"`
	// A pointer, so a test can tell "no day count sent" from "zero days" —
	// which is the whole distinction the field exists to keep.
	DaysSinceTouch *int `json:"days_since_touch"`
}

type dealCoverageDTO struct {
	DealID       string            `json:"deal_id"`
	Stakeholders []AnyMap          `json:"stakeholders"`
	OurSide      []AnyMap          `json:"our_side"`
	Risks        []coverageRiskDTO `json:"risks"`
}

func TestPersonNetworkAnswersHonestlyWhenNobodyKnowsThem(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var person AnyMap
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Unknown Contact"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("creating the contact: %d", status)
	}
	id, _ := person["id"].(string)

	// A contact nobody has spoken to answers 200 with an EMPTY list, not 404
	// and not an error. "The account is cold" is the useful answer here, and
	// the surface has to be able to say it.
	var got personNetworkDTO
	if status := e.Call(t, "GET", "/v1/people/"+id+"/network", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("network status = %d, want 200", status)
	}
	if got.PersonID != id {
		t.Errorf("payload names person %s, want %s", got.PersonID, id)
	}
	if len(got.Colleagues) != 0 {
		t.Errorf("a contact with no interactions has %d colleagues", len(got.Colleagues))
	}
}

func TestPersonNetworkHidesAContactTheCallerCannotRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// A person id that does not exist is indistinguishable from one this
	// caller may not see — existence is not disclosed, here as everywhere.
	var body AnyMap
	if status := e.Call(t, "GET",
		"/v1/people/019fb000-0000-7000-8000-00000000dead/network", nil, nil, &body); status != http.StatusNotFound {
		t.Errorf("an unknown contact answered %d, want 404 — a network read must not "+
			"confirm that a record exists when the person read would not", status)
	}
}

func TestDealCoverageFlagsAThreadlessDealAndExplainsWhy(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var pipelines AnyMap
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK {
		t.Fatalf("listing pipelines: %d", status)
	}
	pipeline, stage := firstPipelineAndStage(t, pipelines)

	var deal AnyMap
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "Threadless", "pipeline_id": pipeline, "stage_id": stage, "source": "ui",
	}, nil, &deal); status != http.StatusCreated {
		t.Fatalf("creating the deal: %d", status)
	}
	dealID, _ := deal["id"].(string)

	// One captured touch, because the finding is deliberately held back until
	// there is one: engagement needs a two-way exchange, so on a deal nobody
	// has contacted EVERY seat is unengaged by construction and the warning
	// would fire on every deal somebody just created. Seeding the touch keeps
	// this test on the case it is named for — a deal that HAS been worked and
	// is still single-threaded — rather than on the calendar.
	var touch AnyMap
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "note", "body": "Kickoff call notes",
		"links": []AnyMap{{"entity_type": "deal", "entity_id": dealID}},
	}, nil, &touch); status != http.StatusCreated {
		t.Fatalf("capturing a touch on the deal: %d", status)
	}

	var got dealCoverageDTO
	if status := e.Call(t, "GET", "/v1/deals/"+dealID+"/coverage", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("coverage status = %d, want 200", status)
	}
	if got.DealID != dealID {
		t.Errorf("payload names deal %s, want %s", got.DealID, dealID)
	}
	// REPORT-PARAM-1 verbatim: zero engaged contacts is below the floor of
	// two, so a deal with nobody on it reads as single-threaded — the same
	// answer the reporting surface gives, which is the point of reusing the
	// rule rather than inventing one.
	var found *coverageRiskDTO
	for i := range got.Risks {
		if got.Risks[i].Kind == "single_threaded_theirs" {
			found = &got.Risks[i]
		}
	}
	if found == nil {
		t.Fatalf("a deal with no engaged contacts raised no single-threaded risk: %+v", got.Risks)
	}
	if found.Summary == "" {
		t.Error("the risk carries no reason — a flag a human cannot read is a red dot")
	}
}

func TestDealCoverageHidesADealTheCallerCannotRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// The coverage payload names the deal's people, so a caller who cannot
	// read the deal must not learn who sits on it.
	var body AnyMap
	if status := e.Call(t, "GET",
		"/v1/deals/019fb000-0000-7000-8000-00000000beef/coverage", nil, nil, &body); status != http.StatusNotFound {
		t.Errorf("an unknown deal answered %d, want 404", status)
	}
}

func TestDealCoverageDistinguishesADepartedChampionFromADepartedStakeholder(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	org, deal := dealAtAnAccount(t, e, "Bär Pharma", "Renewal")

	// Two people who used to work there and one who still does. The rule
	// demands EVIDENCE of a departure — an ended employment plus no live one —
	// so the third person proves the flag is about leaving and not about
	// having no employment row.
	gone := contactAt(t, e, org, "Departed Champion", "2026-01-31")
	alsoGone := contactAt(t, e, org, "Departed Legal", "2026-02-28")
	stayed := contactAt(t, e, org, "Still There", "")

	stakeholder(t, e, deal, gone, "champion")
	stakeholder(t, e, deal, alsoGone, "legal")
	stakeholder(t, e, deal, stayed, "user")

	risks := coverageRisks(t, e, deal)
	champion := findRisk(risks, "champion_left")
	if champion == nil {
		t.Fatalf("the champion left the account and no champion_left risk fired: %+v", risks)
	}
	if len(champion.PersonIDs) != 1 || champion.PersonIDs[0] != gone {
		t.Errorf("champion_left names %v, want only the departed champion %s", champion.PersonIDs, gone)
	}

	stakeholder := findRisk(risks, "stakeholder_left")
	if stakeholder == nil {
		t.Fatalf("a non-champion seat left the account and no stakeholder_left risk fired: %+v", risks)
	}
	// The colleague who still works there must NOT appear. A departure list
	// that swept in every seat would put a resignation on a person who is at
	// their desk.
	for _, id := range stakeholder.PersonIDs {
		if id == stayed {
			t.Errorf("a stakeholder who still works at the account was reported as having left")
		}
		if id == gone {
			t.Errorf("the champion was reported twice — once as champion_left and once as stakeholder_left")
		}
	}
}

func TestDealCoverageDoesNotCallARehiredStakeholderDeparted(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	org, deal := dealAtAnAccount(t, e, "Voss Systems", "Expansion")

	// A role change recorded as end-then-start, which is how most CRMs record
	// a promotion. The closed row is real and so is the live one, and only the
	// live one decides: flagging this would announce a resignation every time
	// somebody changed job title.
	person := contactAt(t, e, org, "Promoted Person", "2026-01-31")
	employ(t, e, person, org, "2026-02-01", "")
	stakeholder(t, e, deal, person, "champion")

	risks := coverageRisks(t, e, deal)
	if findRisk(risks, "champion_left") != nil {
		t.Errorf("a stakeholder with a live employment was reported as departed: %+v", risks)
	}
}

func TestGoingColdFiresOnTheReportingWindowAndCarriesTheDayCount(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	_, deal := dealAtAnAccount(t, e, "Kessler GmbH", "Silent")

	// A brand-new deal has been silent since it was created, which is zero
	// days — not cold. The flag must not fire on the day a rep writes a deal
	// down.
	if findRisk(coverageRisks(t, e, deal), "going_cold") != nil {
		t.Error("a deal created moments ago was flagged as going cold")
	}

	// Age the last touch past REPORT-PARAM-2's window. Written through the
	// owner connection because no API sets this column — it is maintained by
	// the capture path, and the point of the test is the read, not the write.
	ageLastTouch(t, e, deal, 41)

	risks := coverageRisks(t, e, deal)
	cold := findRisk(risks, "going_cold")
	if cold == nil {
		t.Fatalf("a deal untouched for 41 days raised no going_cold risk: %+v", risks)
	}
	if cold.DaysSinceTouch == nil {
		t.Fatal("going_cold carries no day count — the 30/60-day views filter on it")
	}
	if *cold.DaysSinceTouch != 41 {
		t.Errorf("going_cold reports %d days, want 41", *cold.DaysSinceTouch)
	}
	// Every other finding leaves the count ABSENT. A zero would read as
	// "touched today" on a rule that says nothing about recency.
	for _, r := range risks {
		if r.Kind != "going_cold" && r.DaysSinceTouch != nil {
			t.Errorf("%s carries days_since_touch=%d; only going_cold speaks about recency",
				r.Kind, *r.DaysSinceTouch)
		}
	}
}

func TestAWonDealIsNotGoingCold(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	_, deal := dealAtAnAccount(t, e, "Brandt Automotive", "Delivered")
	ageLastTouch(t, e, deal, 400)
	closeWon(t, e, deal)

	// Silence after the close is delivery, not decay. Flagging it would train
	// a rep to ignore the flag on the deals where it means something.
	if findRisk(coverageRisks(t, e, deal), "going_cold") != nil {
		t.Error("a won deal silent for 400 days was flagged as going cold")
	}
}

// --- fixture helpers ---

// coverageRisks reads one deal's findings through the real endpoint.
func coverageRisks(t *testing.T, e *apptest.AppEnv, dealID string) []coverageRiskDTO {
	t.Helper()
	var got dealCoverageDTO
	if status := e.Call(t, "GET", "/v1/deals/"+dealID+"/coverage", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("coverage status = %d, want 200", status)
	}
	return got.Risks
}

func findRisk(risks []coverageRiskDTO, kind string) *coverageRiskDTO {
	for i := range risks {
		if risks[i].Kind == kind {
			return &risks[i]
		}
	}
	return nil
}

// dealAtAnAccount creates an organization and an open deal owned by it — the
// shape every departure rule needs, since a deal with no account has nobody to
// have left.
func dealAtAnAccount(t *testing.T, e *apptest.AppEnv, orgName, dealName string) (org, deal string) {
	t.Helper()
	var created AnyMap
	if status := e.Call(t, "POST", "/v1/organizations",
		AnyMap{"display_name": orgName, "source": "ui"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("creating the account: %d", status)
	}
	org, _ = created["id"].(string)

	var pipelines AnyMap
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK {
		t.Fatalf("listing pipelines: %d", status)
	}
	pipeline, stage := firstPipelineAndStage(t, pipelines)

	var made AnyMap
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": dealName, "pipeline_id": pipeline, "stage_id": stage,
		"organization_id": org, "source": "ui",
	}, nil, &made); status != http.StatusCreated {
		t.Fatalf("creating the deal: %d", status)
	}
	deal, _ = made["id"].(string)
	return org, deal
}

// hiredOn is when every fixture employee started. The start date is not what
// any of these rules read — only whether an employment has ENDED — so one
// shared value keeps the fixtures about the thing under test.
const hiredOn = "2020-01-01"

// contactAt creates a person and their employment at the account. An empty
// endedAt means they still work there.
func contactAt(t *testing.T, e *apptest.AppEnv, org, name, endedAt string) string {
	t.Helper()
	var person AnyMap
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": name}, nil, &person); status != http.StatusCreated {
		t.Fatalf("creating %s: %d", name, status)
	}
	id, _ := person["id"].(string)
	employ(t, e, id, org, hiredOn, endedAt)
	return id
}

func employ(t *testing.T, e *apptest.AppEnv, person, org, startedAt, endedAt string) {
	t.Helper()
	body := AnyMap{
		"kind": "employment", "person_id": person, "organization_id": org,
		"started_at": startedAt, "source": "ui",
	}
	if endedAt != "" {
		body["ended_at"] = endedAt
	}
	var out AnyMap
	if status := e.Call(t, "POST", "/v1/relationships", body, nil, &out); status != http.StatusCreated {
		t.Fatalf("recording employment: %d (%v)", status, out)
	}
}

func stakeholder(t *testing.T, e *apptest.AppEnv, deal, person, role string) {
	t.Helper()
	var out AnyMap
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "deal_stakeholder", "deal_id": deal, "person_id": person,
		"role": role, "source": "ui",
	}, nil, &out); status != http.StatusCreated {
		t.Fatalf("seating the %s stakeholder: %d (%v)", role, status, out)
	}
}

// ageLastTouch backdates the deal's last captured touch. Written as owner
// because no API writes this column — capture maintains it — and the behaviour
// under test is how the coverage read interprets it.
func ageLastTouch(t *testing.T, e *apptest.AppEnv, deal string, days int) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE deal SET last_activity_at = now() - make_interval(days => $2) WHERE id = $1`,
		deal, days); err != nil {
		t.Fatalf("backdating the deal's last touch: %v", err)
	}
}

func closeWon(t *testing.T, e *apptest.AppEnv, deal string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE deal SET status = 'won', closed_at = now() WHERE id = $1`, deal); err != nil {
		t.Fatalf("closing the deal as won: %v", err)
	}
}

// firstPipelineAndStage picks the default pipeline and its first stage.
func firstPipelineAndStage(t *testing.T, listed AnyMap) (pipeline, stage string) {
	t.Helper()
	data, _ := listed["data"].([]any)
	if len(data) == 0 {
		t.Fatal("the workspace seed produced no pipeline")
	}
	first, _ := data[0].(map[string]any)
	pipeline, _ = first["id"].(string)
	stages, _ := first["stages"].([]any)
	if len(stages) == 0 {
		t.Fatal("the default pipeline has no stages")
	}
	firstStage, _ := stages[0].(map[string]any)
	stage, _ = firstStage["id"].(string)
	return pipeline, stage
}
