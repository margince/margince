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

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
)

type sentence = crmcontracts.OrganizationBriefSentence

// composeDeterministic builds the card from the facts alone.
//
// It writes fewer sections than the model does, and that is honest rather than
// lazy: reading what a buyer WANTS out of their own words is not something a
// fold over records can do, and a deterministic guess at it would be the one
// section of this card nobody could check.
func composeDeterministic(f facts, mv crmcontracts.DealStatusCardMove) crmcontracts.DealStatusCard {
	out := crmcontracts.DealStatusCard{
		DealId:      f.deal.Id,
		GeneratedAt: f.now,
		GeneratedBy: crmcontracts.Deterministic,
		Story:       crmcontracts.DealStatusCardSection{Sentences: storyLines(f)},
	}
	if blocker := blockerLines(f); len(blocker) > 0 {
		out.Blocker = &crmcontracts.DealStatusCardSection{Sentences: blocker}
	}
	if mv.Action != ActionNone {
		out.Next = &mv
	}
	// Set on the floor, so the model path inherits it: foldWritten starts from
	// this card, and reply_to is a fact about the records rather than anything
	// the lane may revise.
	if inbound, ok := unansweredInbound(f); ok {
		id := inbound.Id
		out.ReplyTo = &id
	}
	return out
}

// storyLines say what has happened: the deal's own fields, then the last
// contact, then what is still owed.
func storyLines(f facts) []sentence {
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

// blockerLines name what is holding the deal up, and say nothing when nothing
// is.
//
// The floor reads the health factors rather than guessing, and it uses the
// formula's OWN sentences — deals.HealthReasons — because a deterministic card
// restates records rather than interpreting them, and those sentences were
// written beside the numbers they explain. The model lane is handed the bare
// measurements instead, for the reason FactorIn gives.
func blockerLines(f facts) []sentence {
	if f.health == nil || !f.health.AtRisk {
		return nil
	}
	var lines []sentence
	for _, r := range deals.HealthReasons(*f.health, f.now) {
		if r.Value >= atRiskFactor {
			continue
		}
		lines = append(lines, plain(r.Reason))
		if len(lines) == maxBlockerRows {
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
// one.
func foldWritten(
	floor crmcontracts.DealStatusCard, w WrittenStatus, f facts, mv crmcontracts.DealStatusCardMove,
) crmcontracts.DealStatusCard {
	out := floor
	out.GeneratedBy = crmcontracts.Model
	out.Story = crmcontracts.DealStatusCardSection{Sentences: wire(w.Story, f)}
	out.Blocker = optionalSection(w.Blocker, f)
	out.Buyer = optionalSection(w.Buyer, f)
	out.Verdict = verdictOf(w, f)
	out.Next = writtenMove(mv, w, f)
	return out
}

// optionalSection is a section the card omits rather than shows empty. A
// heading over no sentence is furniture, and on this card it also reads as a
// claim — an empty "what is holding this up" says nothing is.
func optionalSection(lines []WrittenLine, f facts) *crmcontracts.DealStatusCardSection {
	wired := wire(lines, f)
	if len(wired) == 0 {
		return nil
	}
	return &crmcontracts.DealStatusCardSection{Sentences: wired}
}

// verdictOf carries the call only when it has both a recognised standing and
// something to rest on. A call with no reasoning is an opinion the reader
// cannot argue with.
func verdictOf(w WrittenStatus, f facts) *crmcontracts.DealStatusCardVerdict {
	because := wire(w.Verdict.Because, f)
	if w.Verdict.Standing == "" || len(because) == 0 {
		return nil
	}
	return &crmcontracts.DealStatusCardVerdict{
		Standing: w.Verdict.Standing,
		Because:  crmcontracts.DealStatusCardSection{Sentences: because},
	}
}

// writtenMove keeps the rules' verb and arguments and takes only the reason.
//
// The model does not draft the words to SEND. That is a drafting surface, and
// this repo has one — compose/draftrules, imported by every surface that
// writes to a counterparty, with a parity test holding them identical. A line
// written here would be a fifth surface outside that block: no sender, no
// recipient, no register, and none of the rules the others learned the hard
// way. The reader drafts through the composer, which has them.
func writtenMove(mv crmcontracts.DealStatusCardMove, w WrittenStatus, f facts) *crmcontracts.DealStatusCardMove {
	if mv.Action == ActionNone {
		return nil
	}
	written := mv
	if w.MoveReason.Text == "" {
		return &written
	}
	// The sentence AND what it rests on. Every other sentence on this card is
	// shown beside the records it cites; a reason shown beside the rules'
	// evidence instead would put a model's words next to a different sentence's
	// sources, which is the one thing a reader following a citation must be
	// able to trust.
	//
	// The rules' evidence stands when the model's citation names a record this
	// build no longer holds: the sentence is still the model's, but there is
	// nothing to open, and an empty list beside prose reads as uncited.
	cited := moveEvidence(w.MoveReason, f)
	if len(cited) == 0 {
		return &written
	}
	written.Reason = w.MoveReason.Text
	written.Evidence = cited
	return &written
}

// moveEvidence resolves the reason's citations to the card's own evidence
// shape. A citation naming a row this build no longer holds is dropped, exactly
// as wire drops one — the filter admitted an id, and only the read here can say
// whether the record is still there.
func moveEvidence(reason WrittenLine, f facts) []crmcontracts.DealNextBestActionEvidence {
	out := make([]crmcontracts.DealNextBestActionEvidence, 0, len(reason.Evidence))
	for _, id := range reason.Evidence {
		if a, ok := citedRecord(f, id); ok {
			out = append(out, evidenceOf(a, subjectOf(a)))
		}
	}
	return out
}

// wire turns the lane's cited lines into sentences the reader can follow back
// to a record. A citation naming a row this build no longer holds is dropped;
// a sentence left with none goes with it, because an uncited claim is the one
// thing the grounding rule exists to prevent.
//
// Every id the filter admits must resolve HERE. A citation the filter accepts
// and this cannot render drops the sentence after it survived grounding, which
// is worse than refusing it: the card loses its best-grounded line silently and
// can end up telling no story at all.
func wire(lines []WrittenLine, f facts) []sentence {
	out := make([]sentence, 0, len(lines))
	for _, line := range lines {
		evidence := make([]crmcontracts.OrganizationBriefEvidence, 0, len(line.Evidence))
		for _, id := range line.Evidence {
			if a, ok := citedRecord(f, id); ok {
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

// citedRecord finds the record one citation names. An open task is an activity
// row like any other, so it cites as one and the reader opens it the same way.
func citedRecord(f facts, id string) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if a.Id.String() == id {
			return a, true
		}
	}
	for _, t := range f.openTasks {
		if t.ID.String() == id {
			return taskAsActivity(t), true
		}
	}
	return crmcontracts.Activity{}, false
}

// taskAsActivity renders an open task as the activity row it is, so a sentence
// resting on a promise cites something the reader can open.
func taskAsActivity(t activities.OpenTask) crmcontracts.Activity {
	subject := t.Subject
	out := crmcontracts.Activity{
		Id:      openapi_types.UUID(t.ID),
		Kind:    crmcontracts.ActivityKindTask,
		Subject: &subject,
	}
	if t.DueAt != nil {
		out.OccurredAt = *t.DueAt
	}
	return out
}
