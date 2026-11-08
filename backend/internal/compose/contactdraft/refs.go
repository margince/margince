// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"slices"

	"github.com/margince/margince/backend/internal/compose/draftcore"
)

// asShown is the input as the model reads it, each record id replaced by its
// ref, and the table a cited ref reads back through. The prompt and the parse
// both call it, so a ref names the same record in both.
func (in Input) asShown() (Input, *draftcore.RecordRefs) {
	refs := draftcore.NewRecordRefs()
	in.Recipient.ID = refs.For(string(in.Recipient.Kind), in.Recipient.ID)
	if in.Deal != nil {
		deal := *in.Deal
		deal.ID = refs.For(citeDeal, deal.ID)
		in.Deal = &deal
	}
	in.Claims = slices.Clone(in.Claims)
	for i := range in.Claims {
		in.Claims[i].SourceID = refs.For(citeActivity, in.Claims[i].SourceID)
	}
	if in.About != nil {
		about := *in.About
		about.ID = refs.For(citeActivity, about.ID)
		about.DealID = refs.For(citeDeal, about.DealID)
		in.About = &about
	}
	in.Recent = slices.Clone(in.Recent)
	for i := range in.Recent {
		in.Recent[i].ID = refs.For(citeActivity, in.Recent[i].ID)
	}
	return in, refs
}
