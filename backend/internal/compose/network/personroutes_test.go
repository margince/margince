// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The list and the single recommendation are one ranking. If they can diverge,
// the card recommends one colleague and lists a different one first — and the
// reader has no way to tell which is the product's actual advice.
func TestTheRecommendationIsTheHeadOfTheList(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			colleague(1, "Thin but direct", crmcontracts.PersonGraphNodeGroupDirect),
			colleague(2, "Busy Ben", crmcontracts.PersonGraphNodeGroupAccount),
			contactNode(3, "Their colleague"),
		},
		[]crmcontracts.PersonGraphEdge{
			{
				From: userNodeID(uuidFor(1)), To: anchor,
				Interactions90d: 2, Inbound90d: intp(1), Outbound90d: intp(1), LastAt: daysBefore(20),
			},
			{
				From: userNodeID(uuidFor(2)), To: personNodeID(uuidFor(3)),
				Interactions90d: 40, Inbound90d: intp(20), Outbound90d: intp(20), LastAt: daysBefore(1),
			},
		})

	routes := chooseRoutes(graph, graphNow)
	route := chooseRoute(routes)
	if len(routes) != 2 {
		t.Fatalf("two askable routes produced %d candidates", len(routes))
	}
	if route == nil {
		t.Fatal("candidates were found and nothing was recommended")
	}
	if routes[0].ViaDisplayName != route.ViaDisplayName {
		t.Errorf("the list leads with %q and the card recommends %q",
			routes[0].ViaDisplayName, route.ViaDisplayName)
	}
}

// Two of their people talking to each other is a fact about the account, not a
// way into it: there is nobody on our side of that edge to ask. Before this
// rule the chooser walked such an edge and then returned NO route at all,
// which read on screen as "nobody here can reach them".
func TestAContactToContactEdgeIsNeverARoute(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			contactNode(3, "Their colleague"),
			contactNode(4, "Their other colleague"),
			colleague(1, "Our quiet rep", crmcontracts.PersonGraphNodeGroupDirect),
		},
		[]crmcontracts.PersonGraphEdge{
			// Loud, recent, and entirely on their side of the wall.
			{
				From: personNodeID(uuidFor(4)), To: personNodeID(uuidFor(3)),
				Interactions90d: 90, Inbound90d: intp(45), Outbound90d: intp(45), LastAt: daysBefore(1),
			},
			{
				From: userNodeID(uuidFor(1)), To: anchor,
				Interactions90d: 3, Inbound90d: intp(1), Outbound90d: intp(2), LastAt: daysBefore(30),
			},
		})

	routes := chooseRoutes(graph, graphNow)
	if len(routes) != 1 {
		t.Fatalf("expected only the colleague's route, got %d", len(routes))
	}
	if routes[0].ViaDisplayName != "Our quiet rep" {
		t.Errorf("route goes via %q; a pair of their own people is not a door",
			routes[0].ViaDisplayName)
	}
	if route := chooseRoute(routes); route == nil {
		t.Error("a usable direct route existed and the card recommended nothing")
	}
}

// A route names the parts a write needs. Parsing the id back apart would make
// a display string load-bearing.
func TestARouteIdSaysWhichKindItIs(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			colleague(1, "Direct Dana", crmcontracts.PersonGraphNodeGroupDirect),
			colleague(2, "Busy Ben", crmcontracts.PersonGraphNodeGroupAccount),
			contactNode(3, "Their colleague"),
		},
		[]crmcontracts.PersonGraphEdge{
			{
				From: userNodeID(uuidFor(1)), To: anchor,
				Interactions90d: 8, Inbound90d: intp(4), Outbound90d: intp(4), LastAt: daysBefore(2),
			},
			{
				From: userNodeID(uuidFor(2)), To: personNodeID(uuidFor(3)),
				Interactions90d: 4, Inbound90d: intp(2), Outbound90d: intp(2), LastAt: daysBefore(6),
			},
		})

	routes := chooseRoutes(graph, graphNow)
	if len(routes) != 2 {
		t.Fatalf("expected both routes, got %d", len(routes))
	}
	direct, indirect := routes[0], routes[1]
	if direct.RouteType != crmcontracts.PersonGraphRouteTypeDirect {
		t.Errorf("the direct route is typed %q", direct.RouteType)
	}
	if direct.ThroughPersonId != nil {
		t.Error("a direct route named an intermediary")
	}
	if indirect.RouteType != crmcontracts.PersonGraphRouteTypeThroughContact {
		t.Errorf("the indirect route is typed %q", indirect.RouteType)
	}
	if indirect.ThroughPersonId == nil {
		t.Fatal("an indirect route did not say who it goes through")
	}
	if indirect.ThroughDisplayName == nil || *indirect.ThroughDisplayName != "Their colleague" {
		t.Error("the indirect route did not name the intermediary a reader has to recognise")
	}
}

// Correspondence is disclosable as counts where the messages themselves are
// not. An indirect route carries the numbers and no rows — the same rule the
// edge itself obeys.
func TestOnlyADirectRouteCarriesReceipts(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	receipts := []crmcontracts.PersonGraphReceipt{{Subject: strptr("Retrofit review")}}
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			colleague(1, "Direct Dana", crmcontracts.PersonGraphNodeGroupDirect),
			colleague(2, "Busy Ben", crmcontracts.PersonGraphNodeGroupAccount),
			contactNode(3, "Their colleague"),
		},
		[]crmcontracts.PersonGraphEdge{
			{
				From: userNodeID(uuidFor(1)), To: anchor, Receipts: &receipts,
				Interactions90d: 8, Inbound90d: intp(4), Outbound90d: intp(4), LastAt: daysBefore(2),
			},
			{
				From: userNodeID(uuidFor(2)), To: personNodeID(uuidFor(3)), Receipts: &receipts,
				Interactions90d: 4, Inbound90d: intp(2), Outbound90d: intp(2), LastAt: daysBefore(6),
			},
		})

	routes := chooseRoutes(graph, graphNow)
	if routes[0].Receipts == nil || len(*routes[0].Receipts) != 1 {
		t.Error("the direct route dropped the proof behind it")
	}
	if routes[1].Receipts != nil {
		t.Error("an indirect route disclosed correspondence it only has counts for")
	}
}

// The facts cross the wire; the sentence is the client's to write, because
// this server speaks English and the product speaks three languages.
func TestEvidenceCrossesTheWireAsFactsNotProse(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			colleague(1, "Direct Dana", crmcontracts.PersonGraphNodeGroupDirect),
		},
		[]crmcontracts.PersonGraphEdge{{
			From: userNodeID(uuidFor(1)), To: anchor,
			Interactions90d: 6, Inbound90d: intp(3), Outbound90d: intp(3), LastAt: daysBefore(2),
		}})

	routes := chooseRoutes(graph, graphNow)
	ev := routes[0].Evidence
	if !ev.TwoWay {
		t.Error("three in and three out did not read as two-way")
	}
	if ev.Interactions90d != 6 {
		t.Errorf("the window count arrived as %d", ev.Interactions90d)
	}
	if ev.DaysSinceLast == nil || *ev.DaysSinceLast != 2 {
		t.Error("the client would have to re-derive today from a timestamp")
	}
}

// A relationship that answers beats one that only sends, whatever the volume.
// The list has to agree with the recommendation on this, not just the head.
func TestAnExchangeOutranksAOneSidedRelationshipInTheList(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			colleague(1, "Talks at them", crmcontracts.PersonGraphNodeGroupDirect),
			colleague(2, "Actually converses", crmcontracts.PersonGraphNodeGroupDirect),
		},
		[]crmcontracts.PersonGraphEdge{
			{
				From: userNodeID(uuidFor(1)), To: anchor,
				Interactions90d: 30, Inbound90d: intp(0), Outbound90d: intp(30), LastAt: daysBefore(1),
			},
			{
				From: userNodeID(uuidFor(2)), To: anchor,
				Interactions90d: 6, Inbound90d: intp(3), Outbound90d: intp(3), LastAt: daysBefore(3),
			},
		})

	routes := chooseRoutes(graph, graphNow)
	if routes[0].ViaDisplayName != "Actually converses" {
		t.Errorf("the list leads with %q; thirty unanswered sends are not a relationship",
			routes[0].ViaDisplayName)
	}
}

// Nothing has been asked for until the introductions module says so. Until
// that seam is bound, offering every route is the honest answer — the opposite
// default would grey out doors that are in fact open.
func TestEveryRouteIsOfferableBeforeAnythingHasBeenAsked(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	graph := graphWith(
		[]crmcontracts.PersonGraphNode{
			colleague(1, "Direct Dana", crmcontracts.PersonGraphNodeGroupDirect),
		},
		[]crmcontracts.PersonGraphEdge{{
			From: userNodeID(uuidFor(1)), To: anchor,
			Interactions90d: 6, Inbound90d: intp(3), Outbound90d: intp(3), LastAt: daysBefore(2),
		}})

	routes := chooseRoutes(graph, graphNow)
	if routes[0].Availability != crmcontracts.PersonGraphRouteAvailabilityAvailable {
		t.Errorf("a route nobody has asked about reads as %q", routes[0].Availability)
	}
}

// Past a handful nobody is choosing a route, they are reading an org chart —
// and the tail is always the coldest.
func TestTheListStopsAtTheCap(t *testing.T) {
	anchor := personNodeID(uuidFor(9))
	nodes := []crmcontracts.PersonGraphNode{}
	edges := []crmcontracts.PersonGraphEdge{}
	for i := range byte(routeCandidateCap + 3) {
		nodes = append(nodes, colleague(i+10, "Colleague", crmcontracts.PersonGraphNodeGroupDirect))
		edges = append(edges, crmcontracts.PersonGraphEdge{
			From: userNodeID(uuidFor(i + 10)), To: anchor,
			Interactions90d: int(i) + 1, Inbound90d: intp(1), Outbound90d: intp(1), LastAt: daysBefore(2),
		})
	}

	if got := len(chooseRoutes(graphWith(nodes, edges), graphNow)); got != routeCandidateCap {
		t.Errorf("the list offered %d routes; the cap is %d", got, routeCandidateCap)
	}
}

func strptr(s string) *string { return &s }
