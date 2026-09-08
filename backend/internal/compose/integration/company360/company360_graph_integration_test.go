// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The connections card's SHAPE: what one hop from an account means, which way
// each edge runs, and the transport that serves it. What the card may not show
// — the per-group grants and the row-scope prune — is pinned next door in
// company360_graphauthz_integration_test.go. This file also holds the fixtures and
// assertions both use.
//
// The placement rules the graph applies to already-read rows (order, caps,
// dropped_count) need no database and live in compose/company360/graph_test.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withSignalRead copies a permission set and adds the signal read grant the
// intro path needs. The harness fixtures deliberately do not carry it — the
// warm room is its own governed surface — so a graph test that wants a path
// has to ask for it, which is also what proves the group is really gated.
func withSignalRead(base principal.Permissions) principal.Permissions {
	perms := base
	perms.Objects = map[string]principal.ObjectGrant{}
	for object, grant := range base.Objects {
		perms.Objects[object] = grant
	}
	perms.Objects["signal"] = principal.ObjectGrant{Read: true}
	return perms
}

// graphRepPerms is the rep the 360 suite uses, plus the signal grant.
// Team-scoped on purpose: an unbounded admin short-circuits every scope
// clause, and the row-scope prune is the whole point of this suite.
var graphRepPerms = withSignalRead(integration.AccountRepPerms)

// graphAdminPerms is the unbounded admin plus the signal grant, for the
// shape tests where row scope is not what is under examination.
var graphAdminPerms = withSignalRead(integration.AdminPerms)

// graphNodeKinds counts the nodes of each kind, which is how a test states
// "the contacts are gone" without depending on their order.
func graphNodeKinds(graph crmcontracts.CompanyGraph) map[crmcontracts.CompanyGraphNodeKind]int {
	out := map[crmcontracts.CompanyGraphNodeKind]int{}
	for _, node := range graph.Nodes {
		out[node.Kind]++
	}
	return out
}

// graphEdgeKinds counts the edges of each kind.
func graphEdgeKinds(graph crmcontracts.CompanyGraph) map[crmcontracts.CompanyGraphEdgeKind]int {
	out := map[crmcontracts.CompanyGraphEdgeKind]int{}
	for _, edge := range graph.Edges {
		out[edge.Kind]++
	}
	return out
}

// graphHasNode reports whether the graph drew one record.
func graphHasNode(graph crmcontracts.CompanyGraph, id ids.UUID) bool {
	return slices.ContainsFunc(graph.Nodes, func(node crmcontracts.CompanyGraphNode) bool {
		return ids.UUID(node.Id) == id
	})
}

// seedOpenSignal records one open, resolved, company-level signal on the
// account — the state the warm room reads as an active path to propose.
func seedOpenSignal(t *testing.T, owner *pgx.Conn, company ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO signal (id, kind, summary, entity_type, entity_id,
		                    resolution_state, resolved_company_id, status, source, captured_by)
		VALUES ($1, 'buying_intent', 'they read the pricing page', 'company', $2,
		        'resolved', $2, 'open', 'manual', 'human:x')`, id, company); err != nil {
		t.Fatalf("seeding a signal: %v", err)
	}
	return id
}

// seedCompanySubjectSignal records an open signal created directly ABOUT the
// account: the subject pair is set and resolved_company_id is NULL, the shape the
// resolver never produces and a hand-created signal always does.
func seedCompanySubjectSignal(t *testing.T, owner *pgx.Conn, company ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO signal (id, kind, summary, entity_type, entity_id,
		                    resolution_state, status, source, captured_by)
		VALUES ($1, 'risk', 'their CFO left', 'company', $2,
		        'resolved', 'open', 'manual', 'human:x')`, id, company); err != nil {
		t.Fatalf("seeding a company-subject signal: %v", err)
	}
	return id
}

// oneHopFixture is an account with one of every edge the card draws, plus two
// records that sit exactly two hops out.
type oneHopFixture struct {
	company, parent, reseller  ids.UUID
	employee, stakeholder      ids.UUID
	deal                       ids.UUID
	grandparent, otherEmployer ids.UUID
}

// seedOneHop builds that account. The two-hop records are seeded here too: a
// suite that only ever seeds what should appear cannot tell a one-hop walk
// from a walk with no limit at all.
func seedOneHop(t *testing.T, e *integration.Env) oneHopFixture {
	t.Helper()
	pipeline, stage, _ := integration.DealFixture(t, e)
	f := oneHopFixture{
		parent:   e.SeedCompany(t, "Holding", &e.Rep1),
		company:  e.SeedCompany(t, "Acme", &e.Rep1),
		reseller: e.SeedCompany(t, "Reseller", &e.Rep1),
	}
	e.WsExec(t, `UPDATE company SET parent_company_id = $2 WHERE id = $1`, f.company, f.parent)
	e.WsExec(t, `INSERT INTO relationship (kind, company_id, counterparty_company_id, source, captured_by)
		VALUES ('partner_of', $1, $2, 'manual', 'human:x')`, f.company, f.reseller)

	f.employee = e.SeedPerson(t, "Dana Buyer", &e.Rep1)
	employ(t, e, f.employee, f.company, "cto")
	f.deal = e.SeedDeal(t, "Renewal", pipeline, stage, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, f.deal, f.company)
	f.stakeholder = e.SeedPerson(t, "Outside Counsel", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'champion', 'manual', 'human:x')`, f.stakeholder, f.deal)

	// Two hops out: the parent's OWN parent, and a company that employs our
	// contact elsewhere.
	f.grandparent = e.SeedCompany(t, "Grand Holding", &e.Rep1)
	e.WsExec(t, `UPDATE company SET parent_company_id = $2 WHERE id = $1`, f.parent, f.grandparent)
	f.otherEmployer = e.SeedCompany(t, "Side Gig", &e.Rep1)
	employ(t, e, f.employee, f.otherEmployer, "advisor")
	return f
}

// TestCompanyGraphCentresOnTheAccountAndWalksOneHop is the shape claim:
// the account is the root node, its employees, open deals, deal stakeholders,
// parent and partner all arrive, and nothing two hops out does.
func TestCompanyGraphCentresOnTheAccountAndWalksOneHop(t *testing.T) {
	e := integration.Setup(t)
	f := seedOneHop(t, e)

	graph, err := company360Service(e).Graph(e.As(e.Rep1, nil, graphAdminPerms),
		ids.From[ids.CompanyKind](f.company))
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if ids.UUID(graph.RootId) != f.company {
		t.Errorf("root_id = %v, want the account %v", graph.RootId, f.company)
	}
	if !graph.AsOf.Equal(company360Clock) {
		t.Errorf("as_of = %v, want the read's pinned instant %v", graph.AsOf, company360Clock)
	}
	assertExactlyOneRoot(t, graph, f.company)
	for _, want := range []ids.UUID{f.parent, f.reseller, f.employee, f.deal, f.stakeholder} {
		if !graphHasNode(graph, want) {
			t.Errorf("node %v is missing — it is one hop from the account", want)
		}
	}
	for _, forbidden := range []ids.UUID{f.grandparent, f.otherEmployer} {
		if graphHasNode(graph, forbidden) {
			t.Errorf("node %v was drawn — it is two hops out", forbidden)
		}
	}
	assertOneEdgeOfEachKind(t, graph)
	if graph.DroppedCount != 0 {
		t.Errorf("dropped_count = %d, want 0 — nothing here reaches a cap", graph.DroppedCount)
	}
	if len(graph.GroupsOmitted) != 0 {
		t.Errorf("groups_omitted = %v, want none for a fully granted caller", graph.GroupsOmitted)
	}
	assertNoDanglingEdge(t, graph)
}

// assertExactlyOneRoot: the card is an ego view, so a second centre — or none —
// would leave the client with no defined layout.
func assertExactlyOneRoot(t *testing.T, graph crmcontracts.CompanyGraph, company ids.UUID) {
	t.Helper()
	roots := 0
	for _, node := range graph.Nodes {
		if node.Root {
			roots++
			if ids.UUID(node.Id) != company {
				t.Errorf("root node is %v, want %v", node.Id, company)
			}
		}
	}
	if roots != 1 {
		t.Errorf("%d nodes carry root, want exactly 1", roots)
	}
}

// assertOneEdgeOfEachKind: the fixture holds exactly one of every edge kind the
// card can draw, so a kind at any other count means an arm double-counted or
// went missing.
func assertOneEdgeOfEachKind(t *testing.T, graph crmcontracts.CompanyGraph) {
	t.Helper()
	edges := graphEdgeKinds(graph)
	for _, kind := range []crmcontracts.CompanyGraphEdgeKind{
		crmcontracts.CompanyGraphEdgeKindEmployment,
		crmcontracts.CompanyGraphEdgeKindHasDeal,
		crmcontracts.CompanyGraphEdgeKindDealStakeholder,
		crmcontracts.CompanyGraphEdgeKindParentOf,
		crmcontracts.CompanyGraphEdgeKindPartnerOf,
	} {
		if edges[kind] != 1 {
			t.Errorf("%s edges = %d, want 1 (all edges: %v)", kind, edges[kind], edges)
		}
	}
}

// assertNoDanglingEdge: an edge naming a record that is not a node is a hole
// the client would have to render as something, and it has nothing to render.
func assertNoDanglingEdge(t *testing.T, graph crmcontracts.CompanyGraph) {
	t.Helper()
	for _, edge := range graph.Edges {
		if !graphHasNode(graph, ids.UUID(edge.From)) || !graphHasNode(graph, ids.UUID(edge.To)) {
			t.Errorf("edge %v -> %v names a record that is not a node", edge.From, edge.To)
		}
	}
}

// TestCompanyGraphRelatedCapCountsCompaniesNotEdges is the bug a row-based
// cap would have: one company can attach to an account many ways — a parent that
// is also a reseller, a referrer and a co-seller, each recordable more than once
// because the partner edges carry no uniqueness constraint. A cap counting rows
// would spend its whole budget on that company and draw a handful of companies
// where the card allows ten, silently, because dropped_count comes off the whole
// set either way.
func TestCompanyGraphRelatedCapCountsCompaniesNotEdges(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)

	// One company that attaches many ways, named to sort FIRST so a row-based
	// cap would meet it before anything else.
	greedy := e.SeedCompany(t, "AAA Greedy Partner", &e.Rep1)
	for range 30 {
		for _, kind := range []string{"partner_of", "referred_by", "co_sell_with"} {
			e.WsExec(t, `INSERT INTO relationship (kind, company_id, counterparty_company_id, source, captured_by)
				VALUES ($1, $2, $3, 'manual', 'human:x')`, kind, company, greedy)
		}
	}
	// Plus twelve companies attached one plain way each — more than the display
	// cap, so the drop count has something to report.
	for i := range 12 {
		partner := e.SeedCompany(t, fmt.Sprintf("Partner %02d", i), &e.Rep1)
		e.WsExec(t, `INSERT INTO relationship (kind, company_id, counterparty_company_id, source, captured_by)
			VALUES ('partner_of', $1, $2, 'manual', 'human:x')`, company, partner)
	}

	graph, err := svc.Graph(e.As(e.Rep1, nil, graphAdminPerms), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	companies := 0
	for _, node := range graph.Nodes {
		if node.Kind == crmcontracts.CompanyGraphNodeKindCompany && !node.Root {
			companies++
		}
	}
	// Thirteen companies attach; the card draws its full allowance of them.
	if companies != 10 {
		t.Errorf("drew %d related companies, want the full allowance of 10 — the greedy company's %d edges must not starve the others",
			companies, 30*3)
	}
	if !graphHasNode(graph, greedy) {
		t.Error("the greedy company is missing; it sorts first and must be drawn")
	}
	// The 90 duplicate edge records collapse: the same relationship recorded
	// twice is one line on the card.
	kinds := graphEdgeKinds(graph)
	for _, kind := range []crmcontracts.CompanyGraphEdgeKind{
		crmcontracts.CompanyGraphEdgeKindReferredBy,
		crmcontracts.CompanyGraphEdgeKindCoSellWith,
	} {
		if kinds[kind] != 1 {
			t.Errorf("%s edges = %d, want 1 — duplicate records are one edge", kind, kinds[kind])
		}
	}
	if graph.DroppedCount != 3 {
		t.Errorf("dropped_count = %d, want 3 — thirteen companies attach and ten are drawn",
			graph.DroppedCount)
	}
}

// TestCompanyGraphHierarchyEdgePointsParentToChild pins the direction
// against the real column, not against the placement helper: a child drawn as
// its parent's parent would invert the whole card.
func TestCompanyGraphHierarchyEdgePointsParentToChild(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)

	parent := e.SeedCompany(t, "Holding", &e.Rep1)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	child := e.SeedCompany(t, "Subsidiary", &e.Rep1)
	e.WsExec(t, `UPDATE company SET parent_company_id = $2 WHERE id = $1`, company, parent)
	e.WsExec(t, `UPDATE company SET parent_company_id = $2 WHERE id = $1`, child, company)

	graph, err := svc.Graph(e.As(e.Rep1, nil, graphAdminPerms), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	want := map[ids.UUID]ids.UUID{parent: company, company: child}
	seen := map[ids.UUID]ids.UUID{}
	for _, edge := range graph.Edges {
		if edge.Kind != crmcontracts.CompanyGraphEdgeKindParentOf {
			continue
		}
		seen[ids.UUID(edge.From)] = ids.UUID(edge.To)
	}
	if len(seen) != len(want) {
		t.Fatalf("drew %d parent_of edges, want %d", len(seen), len(want))
	}
	for from, to := range want {
		if seen[from] != to {
			t.Errorf("parent_of from %v points at %v, want %v", from, seen[from], to)
		}
	}
}

// The transport is thin, but "thin" is a claim: it has to bind the path id,
// let the service's gates decide, hand back the assembled body — and a native
// workspace must reach it rather than meeting the overlay guard.
func TestCompanyGraphTransportServesANativeWorkspace(t *testing.T) {
	e := integration.Setup(t)
	handlers := company360svc.NewHandlers(company360Service(e),
		func(context.Context) (bool, error) { return false, nil })
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphRepPerms)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/companies/"+company.String()+"/graph", nil)
	handlers.GetCompanyGraph(rec, req.WithContext(rep), crmcontracts.Id(company.UUID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var body crmcontracts.CompanyGraph
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the graph body: %v", err)
	}
	if ids.UUID(body.RootId) != company.UUID {
		t.Errorf("root_id = %v, want %v", body.RootId, company)
	}
	// An account with no contacts, deals or partners still has an owner, so the
	// bare graph is the root plus the one user node the owns edge needs.
	if len(body.Nodes) != 2 {
		t.Fatalf("nodes = %d for a bare account, want the root and its owner", len(body.Nodes))
	}
	if body.Nodes[0].Label != "Acme" {
		t.Errorf("root label = %q, want Acme", body.Nodes[0].Label)
	}
	if body.Nodes[1].Kind != crmcontracts.CompanyGraphNodeKindUser {
		t.Errorf("second node kind = %q, want the account owner's user node", body.Nodes[1].Kind)
	}
	// The three collection fields are always arrays on the wire: a client that
	// had to handle null for "none" would branch on it in three places.
	if body.Nodes == nil || body.Edges == nil || body.GroupsOmitted == nil {
		t.Errorf("nodes = %v, edges = %v, groups_omitted = %v; all three must be arrays, never null",
			body.Nodes, body.Edges, body.GroupsOmitted)
	}
}

// TestCompanyGraphRefusesAnOverlayWorkspace: the mirror holds the
// incumbent's records, not our relationship edges, so there is no honest graph
// to draw from it — one refusal, not a card that quietly omits everything.
func TestCompanyGraphRefusesAnOverlayWorkspace(t *testing.T) {
	e := integration.Setup(t)
	handlers := company360svc.NewHandlers(company360Service(e),
		func(context.Context) (bool, error) { return true, nil })
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/companies/"+company.String()+"/graph", nil)
	handlers.GetCompanyGraph(rec,
		req.WithContext(e.As(e.Rep1, []ids.UUID{e.Team1}, graphRepPerms)), crmcontracts.Id(company.UUID))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "unsupported_in_overlay_mode") {
		t.Errorf("body %s does not carry the overlay refusal code", body)
	}
}
