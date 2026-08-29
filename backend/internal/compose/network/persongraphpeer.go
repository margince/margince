// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The peer arm's budget. Same shape as the direct arm: over-fetch recency-
// ordered rows, rank by strength, draw a handful — past eight acquaintances
// the reader is no longer asking "who do they know", they are reading a list.
const (
	graphPeerCap   = 8
	graphPeerFetch = 50
	// suggestEvidenceFloor is how many shared activities in the scoring window
	// make a pair worth PROPOSING as an asserted edge. Deliberately not the §4
	// band: the score's reciprocity term floors any pair we observed no
	// direction between, so a band threshold would demand twenty shared
	// threads where three already say "these two keep appearing together".
	suggestEvidenceFloor = 3
)

// addPeerGroup draws who else the anchor is observed with — other EXTERNAL
// contacts on the same captured activities, from the contact↔contact
// projection. It is the arm that stops the picture being hub-and-spoke
// through our own team.
//
// No grant of its own and so no withheld state: the person read the anchor
// already required is the only object this arm discloses, and each peer row
// was filtered through the caller's person row scope at source. There is no
// none-band peer to filter: a row only exists here because §4's decay floors
// at BucketWeak for any real LastInteraction, and a ContactEdge cannot exist
// without one — "they were once on a thread together" is exactly what made
// the row, however long ago.
//
// Pooled counts only, no receipts, the account arm's disclosure rule: the
// metadata may be shown where the correspondence behind it may not.
func (h Reads) addPeerGroup(
	ctx context.Context,
	tx pgx.Tx,
	personID ids.PersonID,
	now time.Time,
	out *crmcontracts.PersonGraph,
) (int, error) {
	peers, err := search.ContactEdgesForPerson(ctx, tx, personID.UUID, graphPeerFetch)
	if err != nil {
		return 0, err
	}
	// Which peers are ALREADY recorded as working with the anchor. The read
	// carries the relationship edge gate; a caller refused there simply gets
	// no suggestions — "not yet recorded" is a claim about which edges exist,
	// and it is not this caller's to hear.
	asserted, err := h.people.WorksWithPeers(ctx, tx, personID)
	suggestible := err == nil
	if err != nil && !isDenied(err) {
		return 0, err
	}
	sort.SliceStable(peers, func(i, j int) bool {
		return peers[i].Edge.StrengthOf(now).Strength > peers[j].Edge.StrengthOf(now).Strength
	})
	shown := 0
	qualified := 0
	for _, peer := range peers {
		score := peer.Edge.StrengthOf(now)
		qualified++
		if shown >= graphPeerCap {
			continue
		}
		shown++
		// A suggestion is a READ: the pair keeps appearing together and no
		// live works_with edge exists yet. Nothing is staged — the one click
		// is the rep's own attributed write. Decided BEFORE the node dedupe,
		// because it holds for the pair, not for which arm drew the node.
		suggest := suggestible && peer.Edge.Count90d >= suggestEvidenceFloor && !asserted[peer.Peer]
		// The peer may already be in the picture as an account contact — one
		// human, one node, and this edge hangs off the node that exists. The
		// suggestion rides that node too: the account arm drawing them first
		// does not make the pair less worth recording.
		if hasNode(out, personNodeID(peer.Peer)) {
			if suggest {
				flagNodeSuggested(out, personNodeID(peer.Peer))
			}
		} else {
			pid := openapi_types.UUID(peer.Peer)
			node := crmcontracts.PersonGraphNode{
				Id:       personNodeID(peer.Peer),
				Type:     crmcontracts.PersonGraphNodeTypeContact,
				Group:    crmcontracts.PersonGraphNodeGroupPeer,
				Label:    peer.FullName,
				PersonId: &pid,
			}
			if suggest {
				flag := true
				node.SuggestEdge = &flag
			}
			out.Nodes = append(out.Nodes, node)
		}
		lastAt := peer.Edge.LastAt
		out.Edges = append(out.Edges, crmcontracts.PersonGraphEdge{
			From:            personNodeID(personID.UUID),
			To:              personNodeID(peer.Peer),
			StrengthBucket:  crmcontracts.PersonGraphEdgeStrengthBucket(score.Bucket),
			Interactions90d: peer.Edge.Count90d,
			LastAt:          &lastAt,
		})
	}
	return qualified - shown, nil
}

// flagNodeSuggested sets the suggestion on an already-drawn node.
func flagNodeSuggested(out *crmcontracts.PersonGraph, id string) {
	for i := range out.Nodes {
		if out.Nodes[i].Id == id {
			flag := true
			out.Nodes[i].SuggestEdge = &flag
			return
		}
	}
}
