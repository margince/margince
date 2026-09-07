// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// attachEmailSummaries fills in the canonical email row behind every cited
// message on this page, in one read for the whole response.
//
// Three sections cite conversations and none of them can enrich its own: the
// suggestion rules fire on activities, and both work-attention lanes read a
// commitment out of one. Enriching per section would spend a statement each and
// let the same message arrive with a summary in one card and without one in
// another, which is the drift the shared citation shape exists to end.
//
// It runs last, after readWorkAttention has written the attention rows it
// decorates. A section this caller may not see contributes no citation, so the
// read is already narrowed to what the page actually shows — and the reader's
// own content gate narrows it again.
func (a *assembly) attachEmailSummaries() error {
	return briefevidence.Attach(
		a.ctx,
		briefevidence.InTx(a.tx, activities.EmailSummariesByIDBatch),
		a.evidenceRows(),
	)
}

// evidenceRows is every cited record on this page that could be a message,
// addressed in place so the enrichment writes back into the response.
func (a *assembly) evidenceRows() []briefevidence.Target {
	var targets []briefevidence.Target
	if a.out.Suggestions != nil {
		targets = append(targets, briefevidence.FromSuggestions(*a.out.Suggestions)...)
	}
	if a.out.Deals != nil {
		deals := a.out.Deals.Data
		for i := range deals {
			targets = append(targets, attentionReceipt(deals[i].Attention)...)
		}
	}
	if a.out.Projects != nil {
		projects := *a.out.Projects
		for i := range projects {
			targets = append(targets, attentionReceipt(projects[i].Attention)...)
		}
	}
	return targets
}

// attentionReceipt is the one receipt a work-attention card carries, when it
// carries one.
func attentionReceipt(attention *crmcontracts.Organization360WorkAttention) []briefevidence.Target {
	if attention == nil {
		return nil
	}
	return briefevidence.FromEvidenceRef(attention.SourceEvidence)
}
