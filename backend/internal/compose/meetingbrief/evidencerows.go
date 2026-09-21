// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// meetingEvidence collects a finished brief's citations: the sections' prose,
// and the plan's prepared questions and scenarios, which cite records directly
// rather than through a sentence.
func meetingEvidence(brief *crmcontracts.MeetingBrief) []briefevidence.Target {
	var targets []briefevidence.Target
	for i := range brief.Sections {
		targets = append(targets, briefevidence.FromSentences(brief.Sections[i].Sentences)...)
	}
	if brief.Plan == nil {
		return targets
	}
	for i := range brief.Plan.Questions {
		targets = append(targets, briefevidence.FromEvidence(brief.Plan.Questions[i].Evidence)...)
	}
	for i := range brief.Plan.Scenarios {
		targets = append(targets, briefevidence.FromEvidence(brief.Plan.Scenarios[i].Evidence)...)
	}
	return targets
}
