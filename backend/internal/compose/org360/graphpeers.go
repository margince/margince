// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"sort"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// graphPeerEdgeCap bounds the contact↔contact edges drawn between an
// account's contacts. Fifteen drawn contacts can share over a hundred pairs,
// and past the strongest twenty the picture stops reading as a picture — the
// cap keeps the acquaintances worth drawing, strongest first, and the ones it
// drops are the weakest, not a random tail.
const graphPeerEdgeCap = 20

// placePeerEdges draws which of the DRAWN contacts are observed together —
// the contact↔contact projection, filtered to pairs both ends of which the
// graph already shows. That containment is the arm's authorization: every id
// it hands to the read came out of this caller's own row-scoped contact read,
// and an edge is only ever drawn between two nodes already disclosed.
//
// A pair whose score has decayed to the none band is not drawn: "they were
// once on a thread together" is an archive fact, not an acquaintance.
func (g *graphAssembly) placePeerEdges() error {
	drawn := g.drawnContactIDs()
	// Fewer than two drawn contacts cannot share an edge — and when the
	// contacts group was withheld, drawn is empty, so this arm asks nothing
	// and the partial-graph contract holds: the peer edges vanish WITH the
	// group they describe rather than failing the whole read on a grant the
	// group loop already translated into an omission.
	if len(drawn) < 2 {
		return nil
	}
	edges, err := search.ContactEdgesAmong(g.ctx, g.tx, drawn)
	if err != nil {
		return err
	}
	sort.SliceStable(edges, func(i, j int) bool {
		return edges[i].StrengthOf(g.now).Strength > edges[j].StrengthOf(g.now).Strength
	})
	shown := 0
	for _, edge := range edges {
		score := edge.StrengthOf(g.now)
		if score.Bucket == relstrength.BucketNone {
			continue
		}
		if shown >= graphPeerEdgeCap {
			break
		}
		shown++
		bucket := crmcontracts.OrganizationGraphEdgeStrengthBucket(score.Bucket)
		strength := score.Strength
		g.out.Edges = append(g.out.Edges, crmcontracts.OrganizationGraphEdge{
			From:           openapi_types.UUID(edge.PersonA),
			To:             openapi_types.UUID(edge.PersonB),
			Kind:           crmcontracts.OrganizationGraphEdgeKindCorrespondsWith,
			StrengthBucket: &bucket,
			Strength:       &strength,
		})
	}
	return nil
}
