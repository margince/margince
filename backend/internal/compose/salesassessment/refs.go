// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// shownRefs names the records one request shows: candidate and decided deals
// d1, d2, …, recorded decisions r1, r2, … and deal owners u1, u2, ….
type shownRefs struct {
	deals, decisions, owners *promptref.Refs
}

// asShown is the input as the model reads it, each record id replaced by its
// ref. Request and Parse both call it, so a reply is read in the refs it was
// shown: a record id the request never showed grounds nothing.
func asShown(in Input) (Input, shownRefs) {
	refs := shownRefs{deals: promptref.New("d"), decisions: promptref.New("r"), owners: promptref.New("u")}
	in.Candidates = slices.Clone(in.Candidates)
	for i, candidate := range in.Candidates {
		in.Candidates[i].ID = refFor(refs.deals, candidate.ID)
		in.Candidates[i].OwnerID = refFor(refs.owners, candidate.OwnerID)
	}
	in.Decisions = slices.Clone(in.Decisions)
	for i, decision := range in.Decisions {
		in.Decisions[i].ID = refFor(refs.decisions, decision.ID)
		in.Decisions[i].Deal = refFor(refs.deals, decision.Deal)
		in.Decisions[i].OwnerID = refFor(refs.owners, decision.OwnerID)
	}
	return in, refs
}

// refFor is id's ref, and empty for an empty id: the field names no record.
func refFor(refs *promptref.Refs, id string) string {
	if id == "" {
		return ""
	}
	return refs.For(id)
}

// recordIDs is next as the caller reads it: the deal and its owner by id.
func (refs shownRefs) recordIDs(next *NextDecision) *NextDecision {
	if next == nil {
		return nil
	}
	deal, dealShown := refs.deals.ID(next.DealID)
	owner, ownerShown := refs.owners.ID(next.OwnerID)
	if !dealShown || (!ownerShown && next.OwnerID != "") {
		return nil
	}
	return &NextDecision{DealID: deal, OwnerID: owner, Text: next.Text}
}
