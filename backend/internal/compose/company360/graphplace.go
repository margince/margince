// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// Placing the read rows as nodes and edges: the deterministic order each
// group is drawn in, the caps and what they report, the direction every edge
// runs, and which contact the warm-intro path marks. No SQL here — every rule
// below is provable from already-read rows (graph_test.go), which is why the
// caps and the edge orientation are testable without a database.

import (
	"sort"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// placeContacts adds the employees, strongest relationship first with contact
// id as the tie-break, and counts the ones the cap left out.
//
// A contact whose strength the read did not resolve sorts strictly LAST,
// behind one measured at zero. The two are different facts — "we have no
// relationship here" against "we could not measure one" — and a bare map read
// would collapse them, because a missing entry and a stored 0 look the same.
func (g *graphAssembly) placeContacts() {
	kept := make([]graphContactEdge, len(g.employees))
	copy(kept, g.employees)
	sort.SliceStable(kept, func(i, j int) bool {
		left, leftMeasured := g.strengths[kept[i].contactID]
		right, rightMeasured := g.strengths[kept[j].contactID]
		if leftMeasured != rightMeasured {
			return leftMeasured
		}
		if left.Strength != right.Strength {
			return left.Strength > right.Strength
		}
		return kept[i].contactID.String() < kept[j].contactID.String()
	})
	if len(kept) > graphContactCap {
		kept = kept[:graphContactCap]
	}
	// Counted against the account's true headcount, not against the rows this
	// read fetched: the read is bounded (graphScanCap) and a count taken from
	// what it happened to hold would understate a large account.
	g.out.DroppedCount += g.employeeTotal - len(kept)
	for _, edge := range kept {
		g.addContactNode(edge)
		g.addEdge(g.companyID.UUID, edge.contactID.UUID,
			crmcontracts.CompanyGraphEdgeKindEmployment, edge.role)
	}
}

// keptDeals is the ONE spelling of "the open deals this graph draws" — the
// first graphDealCap of the amount-ordered read. Both readers go through it:
// the seat read, which must ask about exactly the deals that will be drawn,
// and the placement below. Two copies of that slice rule would let seats be
// read for one set and edges drawn for another, and the dangling-edge guard
// would hide the drift.
func (g *graphAssembly) keptDeals() []graphDeal {
	if len(g.openDeals) > graphDealCap {
		return g.openDeals[:graphDealCap]
	}
	return g.openDeals
}

// placeDeals adds the open deals in amount order and, under each, the
// stakeholder seats on it. A seat whose contact the caller cannot read never
// arrived, so no edge here can dangle.
func (g *graphAssembly) placeDeals() {
	kept := g.keptDeals()
	g.out.DroppedCount += g.openDealTotal - len(kept)
	drawn := map[ids.UUID]bool{}
	for _, deal := range kept {
		drawn[deal.dealID] = true
		g.addNode(crmcontracts.CompanyGraphNode{
			Id:     openapi_types.UUID(deal.dealID),
			Kind:   crmcontracts.CompanyGraphNodeKindDeal,
			Label:  deal.name,
			Detail: deal.stageName,
		})
		g.addEdge(g.companyID.UUID, deal.dealID, crmcontracts.CompanyGraphEdgeKindHasDeal, nil)
	}
	for _, seat := range g.seats {
		if !drawn[seat.dealID] {
			continue
		}
		g.addContactNode(seat.contact)
		g.addEdge(seat.dealID, seat.contact.contactID.UUID,
			crmcontracts.CompanyGraphEdgeKindDealStakeholder, seat.role)
	}
}

// graphRelatedCompany is one company one hop away, and how it is attached.
type graphRelatedCompany struct {
	companyID   ids.UUID
	displayName string
	// logoObjectKey is where the company's resolved logo lives, nil when it
	// has none — the node's face, so a related company on the graph is
	// recognized the same way it is on its own record.
	logoObjectKey *string
	// relation is the arm that found it: parent, child, or partner.
	relation string
	// partnerKind is the relationship kind on a partner arm, and edgeOwner
	// is the company that records that edge — the edge points from it to its
	// counterparty, which is a direction the two arms do not share.
	partnerKind *string
	edgeOwner   *ids.UUID
}

// The relation arms, spelled once so the SQL labels and the edge builder
// cannot drift apart.
const (
	graphRelationParent  = "parent"
	graphRelationChild   = "child"
	graphRelationPartner = "partner"
)

// placeRelated caps the account's parent, children and partner counterparties
// and draws them — all one hop, all already pruned to the caller's
// company row scope by the read. Name order with the id as tie-break, so
// two reads of an unchanged account draw the same companies.
func (g *graphAssembly) placeRelated(related []graphRelatedCompany) {
	// Every row given here is drawn: the cap is the READ's, applied to companies
	// rather than to rows, because one company holding many partner edges must
	// not fill a row budget and starve the others. What was left out is the
	// difference against the read's own DISTINCT company count, so a company
	// appearing on three rows is one drop rather than three.
	drawn := map[ids.UUID]bool{}
	for _, row := range related {
		drawn[row.companyID] = true
		g.addNode(crmcontracts.CompanyGraphNode{
			Id:      openapi_types.UUID(row.companyID),
			Kind:    crmcontracts.CompanyGraphNodeKindCompany,
			Label:   row.displayName,
			LogoUrl: contacts.LogoURL(row.companyID, row.logoObjectKey, contacts.LogoWide),
		})
		from, to, kind := g.relatedEdge(row)
		g.addEdge(from, to, kind, nil)
	}
	g.out.DroppedCount += g.relatedTotal - len(drawn)
}

// relatedEdge orients one related company's edge. The hierarchy edge
// always points parent → child; a partner edge always points from the company
// that RECORDS it to its counterparty, which is why the arm carries the
// owner rather than assuming this account is on either side.
func (g *graphAssembly) relatedEdge(row graphRelatedCompany) (ids.UUID, ids.UUID, crmcontracts.CompanyGraphEdgeKind) {
	switch row.relation {
	case graphRelationParent:
		return row.companyID, g.companyID.UUID, crmcontracts.CompanyGraphEdgeKindParentOf
	case graphRelationChild:
		return g.companyID.UUID, row.companyID, crmcontracts.CompanyGraphEdgeKindParentOf
	default:
		from, to := g.companyID.UUID, row.companyID
		if row.edgeOwner != nil && *row.edgeOwner == row.companyID {
			from, to = row.companyID, g.companyID.UUID
		}
		kind := crmcontracts.CompanyGraphEdgeKindPartnerOf
		if row.partnerKind != nil {
			kind = crmcontracts.CompanyGraphEdgeKind(*row.partnerKind)
		}
		return from, to, kind
	}
}

// placeOurSide draws our side of the account: the member who owns it, and the
// colleagues with recorded contact with the contacts the card is showing.
//
// No edge here can dangle: readInContactWith is given the contacts already
// PLACED (drawnContactIDs), so every contact an edge points at is a node before
// this runs — the same already-drawn rule placeDeals applies to a stakeholder
// seat on a dropped deal.
//
// The drop count runs over the colleagues WITH CONTACT only, and it is exact
// because the read chose its capped users over that same placed-contact set.
// The owner is never capped, so counting them in the total would let one drawn
// owner push dropped_count below the contract's `minimum: 0`.
func (g *graphAssembly) placeOurSide() {
	if g.accountOwner != nil {
		g.addUserNode(*g.accountOwner)
		g.addEdge(g.accountOwner.userID, g.companyID.UUID,
			crmcontracts.CompanyGraphEdgeKindOwns, nil)
	}
	drawn := map[ids.UUID]bool{}
	for _, edge := range g.ourSide {
		drawn[edge.user.userID] = true
		g.addUserNode(edge.user)
		g.addContactEdge(edge)
	}
	g.out.DroppedCount += g.ourSideTotal - len(drawn)
}

// addUserNode adds one colleague. A user node carries a name and nothing else:
// §4 measures our relationship with the account's contacts, not with each other,
// and how this colleague is connected is the EDGE's kind. The owner who also
// emailed a contact dedupes through nodeIndex into ONE node with two edges.
func (g *graphAssembly) addUserNode(user graphUser) {
	g.addNode(crmcontracts.CompanyGraphNode{
		Id:    openapi_types.UUID(user.userID),
		Kind:  crmcontracts.CompanyGraphNodeKindUser,
		Label: user.displayName,
	})
}

// markIntroPath names the contact the warm room would route the account's
// active signal through, and marks their node.
//
// It reports nothing unless that exact contact is already a node here. The
// ranking is the warm room's own (signals.RankRouteIn), so the two surfaces
// can only ever name the same contact — and when the card is not showing that
// contact, because their only seat is on a deal it did not draw, saying
// nothing is the honest answer. Naming the strongest contact it happens to
// be showing would be a second, quieter ranking.
func (g *graphAssembly) markIntroPath() {
	if g.signalID == nil {
		return
	}
	ranked := signals.RankRouteIn(g.routeIn, func(contactID ids.ContactID) (int, bool) {
		strength, ok := g.strengths[contactID]
		return strength.Strength, ok
	})
	if len(ranked) == 0 {
		return
	}
	contactID := ranked[0].ContactID.UUID
	index, drawn := g.nodeIndex[contactID]
	if !drawn {
		return
	}
	onPath := true
	g.out.Nodes[index].IntroPath = &onPath
	g.out.IntroPath = &crmcontracts.CompanyGraphIntroPath{
		SignalId:  openapi_types.UUID(*g.signalID),
		ContactId: openapi_types.UUID(contactID),
	}
}

// addContactNode adds one contact, carrying the §4 score their node is
// weighted by. A contact the graph already holds keeps the node it has: the
// employment title is the durable description of who they are, and a
// stakeholder seat arriving later must not overwrite it.
func (g *graphAssembly) addContactNode(edge graphContactEdge) {
	node := crmcontracts.CompanyGraphNode{
		Id:     openapi_types.UUID(edge.contactID.UUID),
		Kind:   crmcontracts.CompanyGraphNodeKindContact,
		Label:  edge.fullName,
		Detail: edge.title,
	}
	if strength, ok := g.strengths[edge.contactID]; ok {
		score := strength.Strength
		bucket := crmcontracts.CompanyGraphNodeStrengthBucket(
			contacts.StrengthBucketToWire(strength.Bucket))
		node.Strength = &score
		node.StrengthBucket = &bucket
	}
	g.addNode(node)
}

// addNode appends a node the graph does not already hold. The first arrival
// wins, so the node order is the deterministic order the groups were placed
// in and a client can lay the graph out the same way twice.
func (g *graphAssembly) addNode(node crmcontracts.CompanyGraphNode) {
	id := ids.UUID(node.Id)
	if _, held := g.nodeIndex[id]; held {
		return
	}
	g.nodeIndex[id] = len(g.out.Nodes)
	g.out.Nodes = append(g.out.Nodes, node)
}

// addEdge appends one edge. Both ends are nodes by construction: every
// caller places the far node first.
// addContactEdge draws one colleague's contact with one contact, carrying how
// warm that particular relationship is.
//
// The band is what a surface renders; the number is what it ranks by. A
// relationship with no qualifying interaction in the window carries the `none`
// band and a NULL number on purpose — "we have never spoken" and "we spoke and
// it went cold" are different facts, and a zero would render them the same.
func (g *graphAssembly) addContactEdge(edge ourSideEdge) {
	bucket := crmcontracts.CompanyGraphEdgeStrengthBucket(edge.strength.Bucket)
	wire := crmcontracts.CompanyGraphEdge{
		From:           openapi_types.UUID(edge.user.userID),
		To:             openapi_types.UUID(edge.contactID),
		Kind:           crmcontracts.CompanyGraphEdgeKindInContactWith,
		StrengthBucket: &bucket,
	}
	if edge.strength.Bucket != relstrength.BucketNone {
		strength := edge.strength.Strength
		wire.Strength = &strength
	}
	g.out.Edges = append(g.out.Edges, wire)
}

func (g *graphAssembly) addEdge(from, to ids.UUID, kind crmcontracts.CompanyGraphEdgeKind, role *string) {
	g.out.Edges = append(g.out.Edges, crmcontracts.CompanyGraphEdge{
		From: openapi_types.UUID(from),
		To:   openapi_types.UUID(to),
		Kind: kind,
		Role: role,
	})
}
