// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"slices"

	"github.com/margince/margince/backend/internal/compose/draftcore"
)

// asShown is the input as the model reads it, each record id replaced by its
// ref, and the table a cited ref reads back through. The prompt and the parse
// both call it, so a ref names the same record in both.
func (in Input) asShown() (Input, *draftcore.RecordRefs) {
	refs := draftcore.NewRecordRefs()
	in.Recipient.ID = refs.For(citeContact, in.Recipient.ID)
	if in.Deal != nil {
		deal := *in.Deal
		deal.ID = refs.For(citeDeal, deal.ID)
		in.Deal = &deal
	}
	if in.Commitment != nil {
		commitment := *in.Commitment
		commitment.ID = refs.For(citeActivity, commitment.ID)
		in.Commitment = &commitment
	}
	in.Recent = slices.Clone(in.Recent)
	for i := range in.Recent {
		in.Recent[i].ID = refs.For(citeActivity, in.Recent[i].ID)
	}
	return in, refs
}
