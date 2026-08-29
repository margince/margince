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
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The peer arm's budget. Same shape as the direct arm: over-fetch recency-
// ordered rows, rank by strength, draw a handful — past eight acquaintances
// the reader is no longer asking "who do they know", they are reading a list.
const (
	graphPeerCap   = 8
	graphPeerFetch = 50
)

// addPeerGroup draws who else the anchor is observed with — other EXTERNAL
// contacts on the same captured activities, from the contact↔contact
// projection. It is the arm that stops the picture being hub-and-spoke
// through our own team.
//
// No grant of its own and so no withheld state: the person read the anchor
// already required is the only object this arm discloses, and each peer row
// was filtered through the caller's person row scope at source. A peer whose
// pair has decayed to the none band is not drawn at all — "they were once on
// a thread together" is an archive fact, not an acquaintance.
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
	sort.SliceStable(peers, func(i, j int) bool {
		return peers[i].Edge.StrengthOf(now).Strength > peers[j].Edge.StrengthOf(now).Strength
	})
	shown := 0
	qualified := 0
	for _, peer := range peers {
		score := peer.Edge.StrengthOf(now)
		if score.Bucket == relstrength.BucketNone {
			continue
		}
		qualified++
		if shown >= graphPeerCap {
			continue
		}
		shown++
		// The peer may already be in the picture as an account contact — one
		// human, one node, and this edge hangs off the node that exists.
		if !hasNode(out, personNodeID(peer.Peer)) {
			pid := openapi_types.UUID(peer.Peer)
			out.Nodes = append(out.Nodes, crmcontracts.PersonGraphNode{
				Id:       personNodeID(peer.Peer),
				Type:     crmcontracts.PersonGraphNodeTypeContact,
				Group:    crmcontracts.PersonGraphNodeGroupPeer,
				Label:    peer.FullName,
				PersonId: &pid,
			})
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
