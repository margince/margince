// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Store) contactCaseViews(ctx context.Context, tx pgx.Tx, ranked []caseSummary) ([]crmcontracts.ContactCase, error) {
	if len(ranked) == 0 {
		return nil, nil
	}
	cases := make([]contactCase, len(ranked))
	which := map[ids.UUID]int{}
	var all []ids.UUID
	for i, summary := range ranked {
		k := summary.component
		cases[i] = contactCase{Anchor: k.anchor, Confidence: k.confidence, TooLarge: k.tooLarge()}
		for _, id := range k.shown(summary.address) {
			which[id] = i
			all = append(all, id)
		}
	}
	members, err := readCaseMembers(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	edges, err := readCaseEdges(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	grants, err := readCaseGrants(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		i := which[member.ID]
		cases[i].Members = append(cases[i].Members, member)
	}
	for _, edge := range edges {
		i := which[edge.Left]
		if i == which[edge.Right] {
			cases[i].Edges = append(cases[i].Edges, edge)
		}
	}
	for _, grant := range grants {
		i := which[grant.RecordID]
		cases[i].Grants = append(cases[i].Grants, grant)
	}
	return s.renderContactCases(ctx, tx, cases, all)
}

func (s *Store) renderContactCases(ctx context.Context, tx pgx.Tx, cases []contactCase, all []ids.UUID) ([]crmcontracts.ContactCase, error) {
	policy, err := config.StoredApplyTx(ctx, tx, s.bootConfig(), SharedMergeReviews)
	if err != nil {
		return nil, err
	}
	writable, err := auth.WritableSubset(ctx, tx, contactEntity, all)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.ContactCase, 0, len(cases))
	for _, c := range cases {
		view, err := renderContactCase(c, policy, caseAuthorityFor(ctx, c, policy, writable))
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}
