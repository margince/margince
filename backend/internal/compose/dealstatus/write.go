// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The deterministic floor: the same card, composed from records.
//
// This runs when no model lane is wired, when the workspace's AI budget is
// spent, or when the grounding filter refuses the reply. It occupies the same
// component and cites the same records — a reader gets a worse-written card,
// never a blank one or a spinner that never resolves, and generated_by says
// which of the two they are reading.
//
// Every sentence restates a record and cites it. That is the floor's whole
// discipline and the reason it can stand in for the model: it says less, but
// nothing it says is invented.

import (
	"fmt"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

type sentence = crmcontracts.OrganizationBriefSentence

// composeDeterministic builds the card from the facts alone.
func composeDeterministic(f facts, mv crmcontracts.DealStatusCardMove) crmcontracts.DealStatusCard {
	out := crmcontracts.DealStatusCard{
		DealId:      dealUUID(f),
		GeneratedAt: f.now,
		GeneratedBy: crmcontracts.Deterministic,
		Standing:    crmcontracts.DealStatusCardSection{Sentences: standingLines(f)},
	}
	if risk := riskLines(f); len(risk) > 0 {
		out.Risk = &crmcontracts.DealStatusCardSection{Sentences: risk}
	}
	if mv.Action != ActionNone {
		out.Next = &mv
	}
	carryHealth(&out, f)
	return out
}

// carryHealth puts the reading on the card so the client shows it without a
// second request. The card reads at_risk rather than the number to decide its
// tone: a threshold the formula owns is not one the frontend should re-derive.
func carryHealth(out *crmcontracts.DealStatusCard, f facts) {
	if f.health == nil {
		return
	}
	health := float32(f.health.Health)
	atRisk := f.health.AtRisk
	out.Health = &health
	out.AtRisk = &atRisk
}

// standingLines say where the deal is: its own fields, then what happened
// last, then what is still owed.
func standingLines(f facts) []sentence {
	lines := []sentence{plain(fmt.Sprintf("%s is %s.", f.deal.Name, f.deal.Status))}
	if last, ok := lastContact(f); ok {
		lines = append(lines, cited(
			fmt.Sprintf("The last contact was %s: %s.", since(f.now, last.OccurredAt), subjectOf(last)),
			last))
	} else {
		lines = append(lines, plain("Nothing has been logged on this deal yet."))
	}
	if n := len(f.openTasks); n > 0 {
		lines = append(lines, plain(openTasksLine(n, f.moreTasks)))
	}
	return lines
}

func openTasksLine(n int, more bool) string {
	if more {
		return fmt.Sprintf("At least %d tasks are still open on this deal.", n)
	}
	if n == 1 {
		return "One task is still open on this deal."
	}
	return fmt.Sprintf("%d tasks are still open on this deal.", n)
}

// riskLines name what is wrong, and say nothing when nothing is. The floor
// reads the health factors rather than guessing: a factor the formula scored
// low is a fact, and the sentence behind it was written beside the formula.
func riskLines(f facts) []sentence {
	if f.health == nil || !f.health.AtRisk {
		return nil
	}
	var lines []sentence
	for _, r := range healthIn(f.health, f.now) {
		if r.Value >= atRiskFactor {
			continue
		}
		lines = append(lines, plain(r.Reason))
		if len(lines) == maxRiskRows {
			break
		}
	}
	return lines
}

// atRiskFactor is the factor value below which the floor names a factor as a
// reason the deal is at risk. The formula owns whether the DEAL is at risk;
// this only decides which of its four parts is worth printing.
const atRiskFactor = 0.5

// plain is a sentence resting on the deal itself — its own fields, which the
// reader is already looking at. It carries no citation because there is no
// second record to open.
func plain(text string) sentence {
	return sentence{Text: text, Evidence: []crmcontracts.OrganizationBriefEvidence{}}
}

// cited is a sentence resting on one activity the reader can open.
func cited(text string, a crmcontracts.Activity) sentence {
	return sentence{Text: text, Evidence: []crmcontracts.OrganizationBriefEvidence{activityEvidence(a)}}
}

// activityEvidence points at one activity, named by its subject where the
// reader may read it. A withheld row is cited by kind: the citation says
// contact happened without repeating words the reader may not see.
func activityEvidence(a crmcontracts.Activity) crmcontracts.OrganizationBriefEvidence {
	name := subjectOf(a)
	return crmcontracts.OrganizationBriefEvidence{
		EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeActivity,
		EntityId:   a.Id,
		Name:       &name,
	}
}

// foldWritten puts the lane's words on the floor's card. The MOVE keeps the
// floor's verb and arguments — the model explains the move, it never chooses
// one — and the health reading stays the formula's.
func foldWritten(
	floor crmcontracts.DealStatusCard, w WrittenStatus, f facts, mv crmcontracts.DealStatusCardMove,
) crmcontracts.DealStatusCard {
	out := floor
	out.GeneratedBy = crmcontracts.Model
	out.Standing = crmcontracts.DealStatusCardSection{Sentences: wire(w.Standing, f)}
	out.Risk = nil
	if len(w.Risk) > 0 {
		out.Risk = &crmcontracts.DealStatusCardSection{Sentences: wire(w.Risk, f)}
	}
	if mv.Action != ActionNone && w.MoveReason != "" {
		written := mv
		written.Reason = w.MoveReason
		out.Next = &written
	}
	return out
}

// wire turns the lane's cited lines into sentences the reader can follow back
// to a record. A citation naming a row this build no longer holds is dropped;
// a sentence left with none goes with it, because an uncited claim is the one
// thing the grounding rule exists to prevent.
func wire(lines []WrittenLine, f facts) []sentence {
	out := make([]sentence, 0, len(lines))
	for _, line := range lines {
		evidence := make([]crmcontracts.OrganizationBriefEvidence, 0, len(line.Evidence))
		for _, id := range line.Evidence {
			if a, ok := timelineRow(f, id); ok {
				evidence = append(evidence, activityEvidence(a))
			}
		}
		if len(evidence) == 0 {
			continue
		}
		out = append(out, sentence{Text: line.Text, Evidence: evidence})
	}
	return out
}

func timelineRow(f facts, id string) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if a.Id.String() == id {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}
