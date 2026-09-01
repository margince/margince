// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// Every way in, not just the warmest one.
//
// The single `route` answers "who should ask"; a rep who cannot use that
// answer — the colleague is on leave, or already said no — needs the next one
// without re-reading the graph and working it out themselves. So the same
// deterministic preference that picks the recommendation orders a list, and
// the recommendation is its head.
//
// Only a COLLEAGUE can carry an introduction. The graph draws contact-to-
// contact edges too, because they explain who talks to whom inside the
// account, but a pair of external contacts is never a route out of here: there
// is nobody on this side of it to ask.

import (
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// routeCandidateCap bounds the list for the same reason the groups are capped:
// past a handful of routes nobody is choosing one, and the tail is always the
// coldest. Matches the cap the account-level intro tool already applies.
const routeCandidateCap = 5

// chooseRoutes orders every askable way in, best first.
//
// The preference is the recommendation's, spelled once: a direct relationship
// beats an indirect one however warm the indirect one looks, then two-way
// beats one-sided, then volume, then the node id so that two identical edges
// never swap places between reads.
func chooseRoutes(out *crmcontracts.PersonGraph, now time.Time) []crmcontracts.PersonGraphRouteCandidate {
	idx := indexNodes(out)
	if idx.anchor == "" {
		return nil
	}

	candidates := []crmcontracts.PersonGraphRouteCandidate{}
	for i := range out.Edges {
		candidate, ok := candidateFor(&out.Edges[i], idx, now)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sortCandidates(candidates)
	if len(candidates) > routeCandidateCap {
		candidates = candidates[:routeCandidateCap]
	}
	return candidates
}

// nodeIndex is the graph read back as lookups, so the ranking reads facts
// about a node rather than re-scanning the slice for each edge.
type nodeIndex struct {
	anchor string
	labels map[string]string
	users  map[string]*openapi_types.UUID
	people map[string]*openapi_types.UUID
}

func indexNodes(out *crmcontracts.PersonGraph) nodeIndex {
	idx := nodeIndex{
		labels: map[string]string{},
		users:  map[string]*openapi_types.UUID{},
		people: map[string]*openapi_types.UUID{},
	}
	for i := range out.Nodes {
		n := &out.Nodes[i]
		idx.labels[n.Id] = n.Label
		idx.users[n.Id] = n.UserId
		idx.people[n.Id] = n.PersonId
		if n.Group == crmcontracts.PersonGraphNodeGroupAnchor {
			idx.anchor = n.Id
		}
	}
	return idx
}

// candidateFor renders one edge as a route, or refuses it.
//
// The refusal is the eligibility rule: an edge whose near end is not a
// colleague describes two of their people talking to each other. It belongs in
// the picture and never in this list, because the list's only verb is "ask
// this person", and there is nobody here to ask.
func candidateFor(
	e *crmcontracts.PersonGraphEdge, idx nodeIndex, now time.Time,
) (crmcontracts.PersonGraphRouteCandidate, bool) {
	via := idx.users[e.From]
	if via == nil {
		return crmcontracts.PersonGraphRouteCandidate{}, false
	}
	candidate := crmcontracts.PersonGraphRouteCandidate{
		RouteType:      crmcontracts.PersonGraphRouteTypeDirect,
		ViaUserId:      *via,
		ViaDisplayName: idx.labels[e.From],
		StrengthBucket: bucketOf(e.StrengthBucket),
		Evidence:       evidenceOf(e, now),
		// Nothing has been asked for until the introductions module says
		// otherwise; a nil availability seam means every route is offerable.
		Availability: crmcontracts.PersonGraphRouteAvailabilityAvailable,
	}
	if e.To == idx.anchor {
		candidate.RouteId = fmt.Sprintf("direct:%s", *via)
		candidate.Receipts = receiptsOf(e)
		return candidate, true
	}

	through := idx.people[e.To]
	if through == nil {
		// An edge that ends on neither the anchor nor a contact is not a way
		// in. Today nothing builds one; refusing is what keeps that true.
		return crmcontracts.PersonGraphRouteCandidate{}, false
	}
	name := idx.labels[e.To]
	candidate.RouteType = crmcontracts.PersonGraphRouteTypeThroughContact
	candidate.RouteId = fmt.Sprintf("through:%s:%s", *via, *through)
	candidate.ThroughPersonId = through
	candidate.ThroughDisplayName = &name
	// No receipts on an indirect route, for the reason the edge itself carries
	// none: the counts are disclosable where the correspondence is not.
	return candidate, true
}

// sortCandidates puts the recommendation first and keeps the order stable.
//
// Insertion sort rather than sort.Slice: the list is capped at a handful of
// routes, and a hand-written comparison that a reader can follow line by line
// is worth more here than an asymptotic gain nobody can measure.
func sortCandidates(candidates []crmcontracts.PersonGraphRouteCandidate) {
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && beatsCandidate(candidates[j], candidates[j-1]); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
}

// beatsCandidate is the whole preference order, in one place.
func beatsCandidate(a, b crmcontracts.PersonGraphRouteCandidate) bool {
	aDirect := a.RouteType == crmcontracts.PersonGraphRouteTypeDirect
	bDirect := b.RouteType == crmcontracts.PersonGraphRouteTypeDirect
	if aDirect != bDirect {
		return aDirect
	}
	if a.Evidence.TwoWay != b.Evidence.TwoWay {
		return a.Evidence.TwoWay
	}
	if a.Evidence.Interactions90d != b.Evidence.Interactions90d {
		return a.Evidence.Interactions90d > b.Evidence.Interactions90d
	}
	// Nothing left that describes the relationship. The id breaks the tie so
	// two equal routes hold their order between reads rather than swapping on
	// map iteration.
	return a.RouteId < b.RouteId
}

// evidenceOf states the counts as facts. The sentence is the client's to
// write: this server speaks English and the product speaks three languages.
func evidenceOf(e *crmcontracts.PersonGraphEdge, now time.Time) crmcontracts.PersonGraphRouteEvidence {
	ev := crmcontracts.PersonGraphRouteEvidence{
		Interactions90d: e.Interactions90d,
		Inbound90d:      e.Inbound90d,
		Outbound90d:     e.Outbound90d,
		TwoWay:          twoWay(e),
	}
	if e.LastAt != nil {
		last := *e.LastAt
		days := elapsed.Days(last, now)
		ev.LastAt = &last
		ev.DaysSinceLast = &days
	}
	return ev
}

// twoWay is the claim that separates a relationship from a mailing list.
func twoWay(e *crmcontracts.PersonGraphEdge) bool {
	return e.Inbound90d != nil && *e.Inbound90d > 0 && e.Outbound90d != nil && *e.Outbound90d > 0
}

func bucketOf(b crmcontracts.PersonGraphEdgeStrengthBucket) *crmcontracts.PersonGraphRouteCandidateStrengthBucket {
	bucket := crmcontracts.PersonGraphRouteCandidateStrengthBucket(b)
	return &bucket
}

func receiptsOf(e *crmcontracts.PersonGraphEdge) *[]crmcontracts.PersonGraphReceipt {
	if e.Receipts == nil {
		return nil
	}
	receipts := *e.Receipts
	return &receipts
}
