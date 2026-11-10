// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionContactCases reads the queue's contact cases through the store the
// review screen lists them from, so a row and the case it opens agree.
type attentionContactCases struct{ store *contacts.Store }

func caseListInput(q attention.CaseQuery) contacts.ContactCaseListInput {
	return contacts.ContactCaseListInput{Scope: q.Scope, Recipient: q.Recipient}
}

// OpenCases reads the ranked census in one snapshot.
func (c attentionContactCases) OpenCases(ctx context.Context, q attention.CaseQuery) ([]attention.ContactCase, error) {
	in := caseListInput(q)
	in.Limit = q.Limit
	page, err := c.store.RankedContactCases(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make([]attention.ContactCase, 0, len(page))
	for _, view := range page {
		out = append(out, laneCase(view))
	}
	return out, nil
}

func (c attentionContactCases) CountCases(ctx context.Context, q attention.CaseQuery) (int, error) {
	return c.store.CountContactCases(ctx, caseListInput(q))
}

// laneCase is what a row shows of a case: its members named, its owners, and
// whether this reader could merge it.
func laneCase(view crmcontracts.ContactCase) attention.ContactCase {
	face := crmcontracts.AttentionContactCase{
		AnchorId: view.AnchorId, Revision: view.Revision, CanMerge: view.CanMerge,
		MergeMode: crmcontracts.AttentionContactCaseMergeMode(view.MergeMode), TooLarge: &view.TooLarge,
		Members: make([]crmcontracts.AttentionPairSide, 0, len(view.Members)),
		Owners:  make([]string, 0, len(view.Owners)),
	}
	for _, m := range view.Members {
		side := crmcontracts.AttentionPairSide{Id: m.Id, Label: m.Name, CreatedAt: &m.CreatedAt}
		side.Detail = m.Email
		if side.Detail == nil {
			side.Detail = m.CompanyName
		}
		face.Members = append(face.Members, side)
	}
	owners := make([]ids.UUID, 0, len(view.Owners))
	for _, o := range view.Owners {
		face.Owners = append(face.Owners, o.Name)
		owners = append(owners, ids.UUID(o.UserId))
	}
	return attention.ContactCase{
		Anchor: ids.UUID(view.AnchorId), Confidence: float64(view.Confidence), Owners: owners, Face: face,
	}
}
