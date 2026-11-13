// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// meetingCitations is every citation in a finished brief: the sections' prose
// and each part of the plan that cites a record. Email summaries and record
// names are both attached through this one walk. A citation it misses is a bare
// "activity" chip with nothing behind it.
//
// Held by: TestEveryCitationInTheBriefIsWalked (backend/internal/compose/meetingbrief/evidencerows_test.go)
func meetingCitations(brief *crmcontracts.MeetingBrief) []*crmcontracts.CompanyBriefEvidence {
	var out []*crmcontracts.CompanyBriefEvidence
	cite := func(evidence []crmcontracts.CompanyBriefEvidence) {
		for i := range evidence {
			out = append(out, &evidence[i])
		}
	}
	for i := range brief.Sections {
		for j := range brief.Sections[i].Sentences {
			cite(brief.Sections[i].Sentences[j].Evidence)
		}
	}
	plan := brief.Plan
	if plan == nil {
		return out
	}
	if plan.Opening != nil {
		cite(plan.Opening.Evidence)
	}
	if plan.Objective != nil {
		cite(plan.Objective.Sentence.Evidence)
	}
	if plan.TopRisk != nil {
		cite(plan.TopRisk.Text.Evidence)
	}
	for _, sentence := range []*crmcontracts.CompanyBriefSentence{&plan.Advance.Best, &plan.Advance.Fallback, &plan.Advance.Minimum} {
		cite(sentence.Evidence)
	}
	for i := range plan.CompanyArc {
		cite(plan.CompanyArc[i].Summary.Evidence)
	}
	for i := range plan.LikelyAsks {
		cite(plan.LikelyAsks[i].Basis.Evidence)
	}
	for i := range plan.Questions {
		cite(plan.Questions[i].Evidence)
	}
	for i := range plan.Scenarios {
		cite(plan.Scenarios[i].Evidence)
	}
	return out
}

func emailTargets(citations []*crmcontracts.CompanyBriefEvidence) []briefevidence.Target {
	var targets []briefevidence.Target
	for _, cited := range citations {
		targets = append(targets, briefevidence.FromEvidenceRef(cited)...)
	}
	return targets
}

// citedActivity is one cited activity as the brief's input states it. The
// time is kept only when the row has one, not just a day.
type citedActivity struct {
	subject string
	at      *time.Time
}

// citedActivities reads names and dates off the rows the brief was written
// from. Every row there already passed this reader's content gate; a history
// row outside their audience is skipped, because its subject is the audience's.
func citedActivities(in Input) map[string]citedActivity {
	out := map[string]citedActivity{}
	add := func(id, subject string, when Occurred) {
		row := out[id]
		if row.subject == "" {
			row.subject = subject
		}
		if row.at == nil && !when.DateOnly && !when.At.IsZero() {
			at := when.At.UTC()
			row.at = &at
		}
		out[id] = row
	}
	add(in.ActivityID, in.Subject, Occurred{At: in.StartsAt, DateOnly: in.StartsDateOnly})
	for _, act := range in.Recent {
		add(act.ID, act.Subject, Occurred{At: act.At, DateOnly: act.DateOnly})
	}
	for _, prior := range in.PriorMeetings {
		add(prior.ID, prior.Subject, prior.StartsAt)
	}
	for _, row := range in.History {
		if !row.Withheld {
			add(row.ID, row.Subject, Occurred{At: row.At, DateOnly: row.DateOnly})
		}
	}
	for _, claim := range in.Commitments {
		var when Occurred
		if claim.OccurredAt != nil {
			when = *claim.OccurredAt
		}
		add(claim.SourceID, claim.SourceLabel, when)
	}
	return out
}

// nameCitations gives each cited activity its subject and date. The chip then
// reads as the conversation it is, and resting on it shows when that was.
func nameCitations(citations []*crmcontracts.CompanyBriefEvidence, in Input) {
	known := citedActivities(in)
	for _, cited := range citations {
		if cited.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
			continue
		}
		row, ok := known[cited.EntityId.String()]
		if !ok {
			continue
		}
		if cited.Name == nil && row.subject != "" {
			cited.Name = &row.subject
		}
		if cited.At == nil {
			cited.At = row.at
		}
	}
}
