// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// Ask Margince: the prepared questions on the company view.
//
// The question is CHOSEN, not typed. Each prepared question names the slice
// of the account its answer may be written from, which is what lets every
// sentence carry a citation the reader can open. A text box would need
// retrieval that can prove what it did NOT find — and a box that quietly
// answered from a subset would read exactly like one that searched
// everything, which is the failure worth avoiding rather than shipping.
//
// Everything below reuses the brief's machinery: the same per-viewer input,
// the same nonce fence around text written outside this workspace, the same
// grounding filter, and the same deterministic floor when no model lane is
// configured. An answer differs from a brief only in which facts it selects
// and what it is asked to say about them.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The prepared questions, derived from the contract's enum rather than
// re-spelled — a rename upstream then fails to compile here instead of
// leaving a question nobody can ask.
var (
	askWhatsOpen    = crmcontracts.CompanyQuestionWhatsOpen
	askMeetingPrep  = crmcontracts.CompanyQuestionMeetingPrep
	askWhatsChanged = crmcontracts.CompanyQuestionWhatsChanged
)

// askInstruction is what each question asks the writer to do. It is the whole
// difference between the three answers, so it is stated once per question and
// nowhere else.
var askInstruction = map[crmcontracts.CompanyQuestion]string{
	askWhatsOpen: "Answer what is currently open on this account: the open deals with their stage and amount, the open tasks, and the open commitments. " +
		"Write one sentence per open item, naming it, who holds it and when it is due. " +
		"Do not speculate about what will close.",
	askMeetingPrep: "Answer what the reader needs before a meeting with this account: who the known contacts are, " +
		"where the pipeline stands, and whether anything is waiting for a reply. Do not invent an agenda.",
	askWhatsChanged: "Answer what has moved on this account recently, newest first, using only the timeline entries in the summary. " +
		"Do not describe a trend the entries do not show.",
}

// ParseQuestion validates the requested question.
//
// An unknown question is a 422 rather than a default, because silently
// answering a different question than the one asked is indistinguishable
// from answering the one asked badly.
func ParseQuestion(raw crmcontracts.CompanyQuestion) (crmcontracts.CompanyQuestion, error) {
	if _, prepared := askInstruction[raw]; !prepared {
		return "", httperr.Validation("question", "unsupported",
			fmt.Sprintf("ask one of the prepared questions: %s, %s or %s",
				askWhatsOpen, askMeetingPrep, askWhatsChanged))
	}
	return raw, nil
}

// askSystem is the shared prompt. The per-question instruction is appended,
// so the grounding rules are stated once and cannot drift between questions.
const askSystem = `You answer one question about one account in a salesperson's CRM, from a JSON summary of that account.
Return ONLY a JSON object: {"sentences":[{"text":"...","evidence":[{"entity_type":"deal|activity|contact|company","entity_id":"..."}]}]}.
Answer in one to four sentences, plainly, addressing the reader as "you" where natural.
State only what the summary states. Never infer a cause, a mood, an intent or a next step it does not contain.
Cite the ids the summary gave you; a sentence about the account itself cites the company.
Put ids ONLY in evidence. An id must never appear in a sentence's text — the reader sees the text, and an id there is unreadable.
Write one claim per sentence, and cite the ONE record that sentence is about. Three records worth naming are three sentences.
If the summary names sections_omitted, leave those subjects out of the answer and say nothing about them at all — the reader is not allowed to see them. A withheld section does not make the question unanswerable: answer from what remains.
If what remains does not answer the question, return an empty sentences array rather than a sentence that talks around it.`

// The answer is filed against the account and read by whoever asks the same
// question next, so it takes the installation's language like the brief above.
func askSystemFor(question crmcontracts.CompanyQuestion, fence promptfence.Fence, lang string) string {
	return askSystem + "\n" + openWorkRule + "\n" + askInstruction[question] + "\n" +
		promptlang.Rule(lang) + "\n" + fence.Rule("account summary")
}

// AskRequest builds the one request a prepared question sends. Exported for
// the same reason BriefRequest is: the certification case must issue the
// request production issues, because a rebuilt copy stays green through the
// change that breaks the original.
// The company profile is withheld here for the same reason BriefRequest
// withholds it: those statements are approved prose, and none of the three
// prepared questions asks about them. A model that had read them could
// paraphrase one into an answer about something else entirely, cited to the
// company and so accepted by the grounding check.
func AskRequest(question crmcontracts.CompanyQuestion, in Input, lang string) model.Request {
	return groundedRequest(func(fence promptfence.Fence, forLang string) string {
		return askSystemFor(question, fence, forLang)
	}, in.withoutProfile(), lang)
}

// Answer writes the answer to one prepared question. lane may be nil, which
// is not an error state: it is the deployment saying this role runs no model,
// and the deterministic floor is the answer.
func Answer(
	ctx context.Context, lane Completer, raw crmcontracts.CompanyQuestion, companyID string, in Input, lang string,
) ([]Sentence, crmcontracts.WrittenBy, error) {
	// Validated HERE, not only in the service: this is exported, so a second
	// caller must not be able to reach the writers with a question the package
	// does not answer. It is what makes deterministicAnswer's default branch
	// unreachable through any path rather than through the paths that remember.
	question, err := ParseQuestion(raw)
	if err != nil {
		return nil, "", err
	}
	deterministic := deterministicAnswer(question, companyID, in, lang)
	if lane == nil {
		return deterministic, crmcontracts.WrittenByDeterministic, nil
	}
	written, modelErr := answerWithModel(ctx, lane, question, companyID, in, lang)
	if modelErr != nil {
		// The declared degrade posture, not a swallowed error: a model that is
		// unavailable, over budget, or answering unparseable JSON must not take
		// the answer down with it, and generated_by reports which the reader
		// got.
		//nolint:nilerr // on_budget_exhausted: degrade — the fallback IS the answer, and generated_by reports it
		return deterministic, crmcontracts.WrittenByDeterministic, nil
	}
	return written, crmcontracts.WrittenByModel, nil
}

func answerWithModel(
	ctx context.Context, lane Completer, question crmcontracts.CompanyQuestion, companyID string, in Input, lang string,
) ([]Sentence, error) {
	resp, err := ai.Ask(ctx, lane, AskRequest(question, in, lang), func(text string) error {
		_, err := ParseBrief(text, companyID, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	kept, err := ParseBrief(resp.Text, companyID, in)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 {
		// A reply that grounded nothing is not the same as "the account has no
		// answer to this": the deterministic floor knows what the summary
		// carries, so it answers instead.
		return nil, errors.New("the answer cited nothing in the account")
	}
	if question != askWhatsOpen {
		return kept, nil
	}
	if missing := uncitedOpenWork(kept, in); missing != "" {
		// A reply that names fewer open records than the input holds would
		// read as the whole list, so the floor answers instead.
		return nil, fmt.Errorf("the answer left out open %s", missing)
	}
	return withOpenWorkGap(kept, companyID, in, companyPhrasesFor(lang)), nil
}

// deterministicAnswer answers without a model, from the same input. Each
// question reads only its own slice of the account, so the floor and the model
// path answer the same question from the same facts.
//
// An empty result is a real outcome, not a failure: a question whose records
// this caller cannot see has no answer, and saying nothing is more honest than
// a sentence written around the gap.
//
// Unexported on purpose, and reached only through Answer, which validates its
// question first — so a question this switch does not handle cannot arrive.
func deterministicAnswer(question crmcontracts.CompanyQuestion, companyID string, in Input, lang string) []Sentence {
	say := companyPhrasesFor(lang)
	var answered []Sentence
	switch question {
	case askWhatsOpen:
		answered = openAnswer(companyID, in, say)
	case askMeetingPrep:
		answered = prepAnswer(companyID, in, say)
	case askWhatsChanged:
		answered = changedAnswer(in, say)
	default:
		// Unreachable: Answer validates against askInstruction, and
		// TestEveryPreparedQuestionCarriesItsOwnInstruction reads the contract's
		// own enum to prove that map covers every declared question. Returning
		// nothing rather than guessing keeps the promise that an answer is
		// written from the records it cites.
		return nil
	}
	return claims.Dedupe(answered)
}

// openAnswer lists what is open, one record per sentence.
//
// The count sentence comes first and states the true total. Then each deal,
// task and commitment gets its own sentence and its own citation. The count
// stays when the list is short, because it carries the pipeline total and the
// won figure, which no per-deal line states.
func openAnswer(companyID string, in Input, say spoken) []Sentence {
	sentences := make([]Sentence, 0, 3*listedRecords+3)
	if len(in.OpenDeals) > 0 {
		sentences = append(sentences, Sentence{Text: pipelineLine(in, say), Evidence: leadDealEvidence(in)})
		sentences = append(sentences,
			perRecordSentences(in.OpenDeals, citeDeal, dealID,
				func(deal DealIn) string { return openDealLine(deal, say) })...)
	}
	if len(in.OpenTasks) > 0 {
		// A single task needs no count in front of it: its own sentence names
		// it and says when it is due, which is everything the count would add.
		if len(in.OpenTasks) > 1 {
			sentences = append(sentences, openTasksLine(in.OpenTasks, say))
		}
		sentences = append(sentences,
			perRecordSentences(in.OpenTasks, citeActivity, taskID,
				func(task TaskIn) string { return openTaskLine(task, say) })...)
	}
	sentences = append(sentences, openCommitmentSentences(in.OpenCommitments, say)...)
	return withOpenWorkGap(sentences, companyID, in, say)
}

// withOpenWorkGap ends the list with a sentence saying it may be shorter than
// the work, when the read was cut or narrowed. Without it a capped list reads as
// "that is everything" and an empty one as "nothing is open".
func withOpenWorkGap(sentences []Sentence, companyID string, in Input, say spoken) []Sentence {
	if !in.OpenWorkCapped && !in.OpenWorkWithheld {
		return sentences
	}
	return append(sentences, Sentence{
		Text:     say.Say(floor.OpenWorkIncomplete),
		Evidence: []Evidence{{EntityType: citeCompany, EntityID: companyID}},
	})
}

// openCommitmentSentences is one sentence per promise. The chip carries the
// claim as its name. Two promises can be read from one message, and the
// message's own subject would then name both.
func openCommitmentSentences(commitments []CommitmentIn, say spoken) []Sentence {
	if len(commitments) == 0 {
		return nil
	}
	sentences := make([]Sentence, 0, listedRecords+1)
	if len(commitments) > 1 {
		sentences = append(sentences, Sentence{
			Text:     countPhrase(len(commitments), say.Say(floor.OpenCommitmentOne), say.Say(floor.OpenCommitmentMany)) + ".",
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: commitments[0].ActivityID, Name: commitments[0].Claim}},
		})
	}
	for i, commitment := range commitments {
		if i == listedRecords {
			break
		}
		sentences = append(sentences, Sentence{
			Text:     openCommitmentLine(commitment, say),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: commitment.ActivityID, Name: commitment.Claim}},
		})
	}
	return sentences
}

func openCommitmentLine(commitment CommitmentIn, say spoken) string {
	due := shortDate(commitment.Due, say)
	if commitment.Side == sideTheirs {
		holder := commitment.Holder
		if holder == "" {
			holder = say.Say(floor.OpenCommitmentUnnamed)
		}
		if due != "" {
			return fmt.Sprintf(say.Say(floor.OpenCommitmentTheirsDue), holder, commitment.Claim, due)
		}
		return fmt.Sprintf(say.Say(floor.OpenCommitmentTheirs), holder, commitment.Claim)
	}
	if due != "" {
		return fmt.Sprintf(say.Say(floor.OpenCommitmentOursDue), commitment.Claim, due)
	}
	return fmt.Sprintf(say.Say(floor.OpenCommitmentOurs), commitment.Claim)
}

// openGroup is one kind of open record and how many of its citations an
// answer to "what is open" must carry.
type openGroup struct {
	// Field names the Input slice, which the test that classifies every Input
	// field reads.
	Field string
	Cites []Evidence
	Need  int
}

// requiredOpenCitations states what a complete open-work answer cites. It is
// every record of a kind when there are at most listedRecords, otherwise
// listedRecords of them. The floor lists that much, so it always passes
// the check built on it. A new kind of open record has to be added here, and a
// test fails until it is.
func requiredOpenCitations(in Input) []openGroup {
	deals := make([]Evidence, 0, len(in.OpenDeals))
	for _, deal := range in.OpenDeals {
		deals = append(deals, Evidence{EntityType: citeDeal, EntityID: deal.ID})
	}
	tasks := make([]Evidence, 0, len(in.OpenTasks))
	for _, task := range in.OpenTasks {
		tasks = append(tasks, Evidence{EntityType: citeActivity, EntityID: task.ID})
	}
	commitments := make([]Evidence, 0, len(in.OpenCommitments))
	for _, commitment := range in.OpenCommitments {
		commitments = append(commitments, Evidence{EntityType: citeActivity, EntityID: commitment.ActivityID})
	}
	groups := []openGroup{
		{Field: "OpenDeals", Cites: deals},
		{Field: "OpenTasks", Cites: tasks},
		{Field: "OpenCommitments", Cites: commitments},
	}
	for i := range groups {
		groups[i].Need = min(len(groups[i].Cites), listedRecords)
	}
	return groups
}

// uncitedOpenWork names the first kind of open record the sentences cite too
// few of, or returns "" when they cite enough.
func uncitedOpenWork(sentences []Sentence, in Input) string {
	// Counted per sentence: two promises read from one message cite the same
	// activity, so each needs a sentence of its own to count as listed.
	citing := map[Evidence]int{}
	for _, sentence := range sentences {
		once := map[Evidence]bool{}
		for _, cite := range sentence.Evidence {
			key := Evidence{EntityType: cite.EntityType, EntityID: cite.EntityID}
			if !once[key] {
				once[key] = true
				citing[key]++
			}
		}
	}
	for _, group := range requiredOpenCitations(in) {
		found := 0
		left := map[Evidence]int{}
		for _, cite := range group.Cites {
			if _, seen := left[cite]; !seen {
				left[cite] = citing[cite]
			}
			if left[cite] > 0 {
				left[cite]--
				found++
			}
		}
		if found < group.Need {
			return group.Field
		}
	}
	return ""
}

func prepAnswer(companyID string, in Input, say spoken) []Sentence {
	sentences := make([]Sentence, 0, listedRecords+4)
	sentences = append(sentences, Sentence{
		Text:     identityLine(in, say),
		Evidence: []Evidence{{EntityType: citeCompany, EntityID: companyID}},
	})
	if len(in.Contacts) > 0 {
		if len(in.Contacts) > 1 {
			sentences = append(sentences, Sentence{
				Text: countPhrase(len(in.Contacts), say.Say(floor.KnownContactOne), say.Say(floor.KnownContactMany)) + ".",
				// The count names nobody, so it cites the contact the list
				// starts with and gives the reader somewhere to open.
				Evidence: []Evidence{{EntityType: citeContact, EntityID: in.Contacts[0].ID}},
			})
		}
		sentences = append(sentences,
			perRecordSentences(in.Contacts, citeContact, contactID,
				func(contact NamedIn) string { return contactLine(contact, say) })...)
	}
	if len(in.OpenDeals) > 0 {
		sentences = append(sentences, Sentence{Text: pipelineLine(in, say), Evidence: leadDealEvidence(in)})
	}
	if len(in.Recent) > 0 {
		sentences = append(sentences, Sentence{
			Text:     lastTouchLine(in.Recent[0], say),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.Recent[0].ID}},
		})
	}
	return sentences
}

// changedAnswer walks the timeline newest-first. Each entry is its own
// sentence citing itself, so the reader can open the one they care about
// instead of trusting a rolled-up count.
func changedAnswer(in Input, say spoken) []Sentence {
	const mostRecent = 3
	sentences := make([]Sentence, 0, mostRecent)
	for i, act := range in.Recent {
		if i >= mostRecent {
			break
		}
		sentences = append(sentences, Sentence{
			Text:     lastTouchLine(act, say),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: act.ID}},
		})
	}
	return sentences
}

func taskID(task TaskIn) string { return task.ID }

func contactID(contact NamedIn) string { return contact.ID }

func contactLine(contact NamedIn, say spoken) string {
	return fmt.Sprintf(say.Say(floor.KnownContactLine), contact.Name)
}

// openTasksLine counts the open tasks and anchors the count on the one that is
// due first.
//
// The count is the TRUE total even when the per-task sentences below it stop at
// listedRecords, because a reader told "5 open tasks" who has nine is worse off
// than one told nothing.
func openTasksLine(tasks []TaskIn, say spoken) Sentence {
	earliest := earliestDue(tasks)
	counted := countPhrase(len(tasks), say.Say(floor.OpenTaskOne), say.Say(floor.OpenTaskMany))
	text := counted + "."
	if due := shortDate(earliest.Due, say); due != "" {
		text = fmt.Sprintf(say.Say(floor.TasksEarliestDue), counted, due)
	}
	return Sentence{Text: text, Evidence: []Evidence{{EntityType: citeActivity, EntityID: earliest.ID}}}
}

// earliestDue picks the task the count sentence is anchored on: the one due
// first, or the one the list starts with when none carries a due date. TaskIn.Due
// is RFC3339 in UTC, so comparing the strings compares the instants.
func earliestDue(tasks []TaskIn) TaskIn {
	earliest := tasks[0]
	for _, task := range tasks[1:] {
		if task.Due == "" {
			continue
		}
		if earliest.Due == "" || task.Due < earliest.Due {
			earliest = task
		}
	}
	return earliest
}

func openTaskLine(task TaskIn, say spoken) string {
	// The subject is quoted for the same reason an activity's is: a task can be
	// raised from mail this workspace did not write, and it must read as theirs.
	due := shortDate(task.Due, say)
	switch {
	case task.Holder != "" && due != "":
		return fmt.Sprintf(say.Say(floor.OpenTaskHeldDue), task.Name, task.Holder, due)
	case task.Holder != "":
		return fmt.Sprintf(say.Say(floor.OpenTaskHeld), task.Name, task.Holder)
	case due != "":
		return fmt.Sprintf(say.Say(floor.OpenTaskDue), task.Name, due)
	}
	return fmt.Sprintf(say.Say(floor.OpenTaskNamed), task.Name)
}

func openDealLine(deal DealIn, say spoken) string {
	line := fmt.Sprintf(say.Say(floor.OpenDealNamed), deal.Name)
	if deal.Stage != "" {
		line += ", " + deal.Stage
	}
	if amount := renderedAmount(deal.AmountMinor, deal.Currency); amount != "" {
		// The same rendering the prompt gets, not a fresh /100 of the minor
		// integer: dividing understates every zero-decimal currency a
		// hundredfold, and two renderings of one number are two chances to
		// disagree. The card formats money properly; this text is the fallback.
		line += ", " + amount + " " + deal.Currency
	}
	if deal.Stalled {
		line += ", " + say.Say(floor.DealStalledMark)
	}
	return line + "."
}
