// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The bypass itself, against real Postgres (#982).
//
// A tier tightened for ONE record type — creating a project confirmed, creating a
// contact not — is honoured by the REST door, whose tier comes from the operation.
// The MCP door resolves a tier from the VERB, and the verb that performs the same
// write is `create_record`, auto-execute for every type it admits.
//
// So the refusal is one tool call away from being irrelevant, and the only way
// to prove that is to let the call reach the write and then look at the
// database. A unit test asserting a resolved tier would prove what the gate
// decided; it would not prove that a project now exists which no human agreed
// to.
//
// The floor these two tests exercise is DECLARED HERE rather than read off the
// contract. It used to be `createProject`/`updateProject`, which the contract
// tightened at the time; ADR-0055 then made both auto-execute and the tests began
// failing for a reason that had nothing to do with what they check. What they
// check is the MECHANISM — that a floor reaches the served registry and is applied
// before admission — and the mechanism does not care which pair carries it. Stating
// the pair here keeps that property provable whatever today's contract declares.
// The gates in agenttierfloor_test.go are the ones that walk the contract.
//
// The registry is built through registryWithGate — the same function the api role
// composes — so what is proved is that the floor is WIRED and applied before
// admission, not merely that the option works. The missing wiring WAS the defect,
// and a registry a test assembled itself would prove only the option.
//
// One seam is stubbed: the authority resolver (adminSeat, from
// dealmovepin_integration_test.go). The harness seeds users with no role
// assignment, so the live resolver denies every grant, and a refusal for a
// missing grant looks exactly like the refusal these tests want. Stubbing the
// question that is NOT under test is what lets the tier be the only thing that
// can answer.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestATierFlooredForOneRecordTypeBindsTheToolDoorToo(t *testing.T) {
	e := integration.Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	native := NewProvider(e.Pool)

	registry := composedRegistryFlooring(e, "project")

	company := seedCompanyForTierFloor(human, t, native)

	// The identical effect POST /v1/projects performs, asked for through the
	// verb whose tier the contract never tightened. Automatic work may not stage
	// the approval the floor asks for. So the floor reaching this door shows as
	// the staging refusal, and nothing is written.
	_, err := registry.Invoke(tierFloorAgent(t, e), "create_record",
		json.RawMessage(`{"record_type":"project","fields":{"name":"Unapproved",`+
			`"company_id":"`+company.String()+`"}}`))
	if !refusedForWantOfAPassport(err) {
		t.Errorf("create_record{project} answered %v, want the floor to hold it back — a tier tightened "+
			"for one record type is a decision the tool door must see", err)
	}
	if n := liveProjectsNamed(human, t, e, "Unapproved"); n != 0 {
		t.Errorf("%d project(s) named Unapproved exist; automatic work performed a write the floor holds back", n)
	}
}

// A floor binds automatic work. An agent on a colleague's passport does what
// that colleague could do unaided, so the same create runs at once.
func TestAFloorDoesNotHoldBackAnAgentOnAColleaguesPassport(t *testing.T) {
	e := integration.Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	registry := composedRegistryFlooring(e, "project")
	company := seedCompanyForTierFloor(human, t, NewProvider(e.Pool))

	if _, err := registry.Invoke(tierFloorAgentOnPassport(t, e), "create_record",
		json.RawMessage(`{"record_type":"project","fields":{"name":"Done at once",`+
			`"company_id":"`+company.String()+`"}}`)); err != nil {
		t.Fatalf("create_record{project} on a colleague's passport → %v, want it run", err)
	}
	if n := liveProjectsNamed(human, t, e, "Done at once"); n != 1 {
		t.Errorf("%d project(s) named Done at once exist, want the one created", n)
	}
}

// The update half of the same bypass. It is a separate call because a fix that
// tightened creates and missed updates would otherwise leave every gate green.
func TestATightenedUpdateAlsoHoldsBackTheToolDoor(t *testing.T) {
	e := integration.Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	native := NewProvider(e.Pool)
	registry := composedRegistryFlooring(e, "project")

	company := seedCompanyForTierFloor(human, t, native)
	project := seedProjectForTierFloor(human, t, native, company)

	_, err := registry.Invoke(tierFloorAgent(t, e), "update_record",
		json.RawMessage(`{"record_type":"project","id":"`+project.String()+
			`","fields":{"name":"Renamed without asking"}}`))
	if !refusedForWantOfAPassport(err) {
		t.Errorf("update_record{project} answered %v, want the floor to hold the patch back", err)
	}
	if n := liveProjectsNamed(human, t, e, "Renamed without asking"); n != 0 {
		t.Error("the project was renamed by automatic work the floor holds back")
	}
}

// An ordinary company patch must NOT stage. `updateCompany` is
// auto-execute, and the confirm-first routes that share (update_record,
// company) — the fact and profile-field corrections — write a sidecar row
// this verb cannot reach. A floor that collapsed them tightened every company patch
// on the tool door while REST kept it automatic: #982 in reverse, which is what
// this test exists to catch and what the first draft of the fix did.
func TestAnOrdinaryCompanyPatchStillRunsUnattended(t *testing.T) {
	e := integration.Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	native := NewProvider(e.Pool)
	registry := composedRegistry(e)

	company := seedCompanyForTierFloor(human, t, native)

	if _, err := registry.Invoke(tierFloorAgent(t, e), "update_record",
		json.RawMessage(`{"record_type":"company","id":"`+company.String()+
			`","fields":{"industry":"Logistics"}}`)); err != nil {
		t.Fatalf("an ordinary company patch answered %v, want it to run — the contract "+
			"declares updateCompany auto-execute, and tightening it here would make the tool "+
			"door stage what the REST door performs", err)
	}
}

// composedRegistry is the api role's own composition, with the authority seam
// stubbed for the reason stated at the top of this file. It goes through
// registryWithGate rather than NewRegistry so the tier floor arrives the way
// production supplies it.
func composedRegistry(e *integration.Env) *agents.Registry {
	// The nils are the readers this test's tools never reach: a drafter, an
	// embedder, a transcript enqueue, an imports reader, a meeting-brief reader
	// and a logger. What it exercises is the tier floor.
	return registryWithGate(e.DB(), auth.NewGate(adminSeat{}), nil,
		SendPath{}, companyEnricher{}, nil, nil, nil, nil, nil, registryFeatures{})
}

// composedRegistryFlooring is that same composition with ONE pair floored to
// confirm-first, for whichever record type the caller names.
//
// The option arrives after registryWithGate's own withContractTierFloor and
// replaces it, which is deliberate: what these tests prove is that a floor
// reaches this registry and runs before admission, and a floor the test states
// cannot go stale when the contract's own declarations move. The tests that walk
// what the contract declares TODAY are the unit gates in agenttierfloor_test.go.
func composedRegistryFlooring(e *integration.Env, recordType string) *agents.Registry {
	registry := composedRegistry(e)
	// Applied AFTER construction, not passed as an option: registryWithGate
	// appends its own withContractTierFloor to whatever it is given, so an
	// option supplied here would be overwritten by the composed one rather than
	// replace it. What is under test is still the composed registry — the same
	// tools, the same gate, the same staging — with one declaration swapped.
	agents.WithTierFloor(func(_, rt string) (mcp.RiskTier, bool) {
		if rt != recordType {
			return mcp.TierAutoExecute, false
		}
		return mcp.TierConfirmationRequired, true
	})(registry)
	return registry
}

// refusedForWantOfAPassport reports the staging engine's refusal of an agent
// with no passport, which is how a floor answers automatic work. The engine
// answers it with a plain error, so its words are matched.
func refusedForWantOfAPassport(err error) bool {
	return err != nil && strings.Contains(err.Error(), "an agent stages on its passport")
}

// seedProjectForTierFloor creates one project through the real writer, so the
// update case patches a row the store actually wrote.
func seedProjectForTierFloor(as context.Context, t *testing.T, p *Provider, company ids.UUID) ids.UUID {
	t.Helper()
	ref, err := p.Create(as, datasource.CreateInput{
		EntityType: datasource.EntityProject,
		Fields:     json.RawMessage(`{"name":"Tier floor project","company_id":"` + company.String() + `"}`),
		Source:     "test",
	})
	if err != nil {
		t.Fatalf("seeding the project: %v", err)
	}
	return ref.ID
}

// seedCompanyForTierFloor creates the anchor company a project requires, through the
// real writer.
func seedCompanyForTierFloor(as context.Context, t *testing.T, p *Provider) ids.UUID {
	t.Helper()
	ref, err := p.Create(as, datasource.CreateInput{
		EntityType: datasource.EntityCompany,
		Fields:     json.RawMessage(`{"display_name":"Tier floor anchor"}`),
		Source:     "test",
	})
	if err != nil {
		t.Fatalf("seeding the anchor company: %v", err)
	}
	return ref.ID
}

// liveProjectsNamed counts the projects the call did or did not create.
//
// Bound through WithWorkspaceTx, not read off the pool directly. Env.Pool is the
// workspace-bound app role, so an unbound `SELECT count(*) FROM project` resolves the
// policy expression against a NULL workspace and answers zero for every row that
// exists — an absence-assertion that passes whether or not the write happened,
// which is the one thing this test may not do.
func liveProjectsNamed(as context.Context, t *testing.T, e *integration.Env, name string) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(as, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(as, `SELECT count(*) FROM project WHERE name = $1`, name).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the projects: %v", err)
	}
	return n
}

// tierFloorAgent is a passport holding every scope and grant a project create
// needs, so the only thing left that can refuse it is the tier.
//
// tierFloorAgent is automatic work: an agent with no passport behind it, which
// a floor holds back.
func tierFloorAgent(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:tier-floor", SeatType: principal.SeatFull,
		OnBehalfOf: e.Rep1, UserID: e.Rep1,
		Scopes:      principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
		Permissions: integration.AdminPerms,
	})
}

// tierFloorAgentOnPassport is the same agent working on a colleague's passport.
// The passport is seeded rather than invented, as the gate reads it as a row.
func tierFloorAgentOnPassport(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	ctx := tierFloorAgent(t, e)
	p, _ := principal.Actor(ctx)
	p.PassportID = e.SeedPassport(t, integration.OwnerConn(t), "tier floor probe")
	return principal.WithActor(ctx, p)
}
