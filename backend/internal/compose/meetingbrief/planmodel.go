// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The plan's own model call.
//
// A SECOND request rather than more fields on the sections one, for three
// reasons that all point the same way. The sections prompt marshals the whole
// Input and asks for up to nine sections inside one output budget; adding ten
// plan fields to that reply roughly doubles the output and one malformed brace
// then takes both halves down. The plan needs facts the Input does not carry —
// the arc, the classified type, the unknowns — so it wants a different
// projection, not a bigger one. And two calls degrade independently: a failed
// plan leaves the sections model-written and each reports its own writer.
//
// WHAT THIS PROJECTION CARRIES is the whole difference between a plan that
// names what an account asked for and one that says "there was a long thread
// about requirements". The excerpts are here, tagged with the ids the model
// must cite. A projection of subject lines would produce exactly the generic
// brief this change exists to replace.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// planSystem is this site's prompt.
const planSystem = `You prepare a salesperson for one meeting, from a JSON briefing about it.
Return ONLY a JSON object: {"objective":{"text":"...","evidence":[{"entity_type":"activity|deal|person","entity_id":"..."}]},"opening":{...},"top_risk":{"text":"...","evidence":[...],"say":"...","show":"...","avoid":"..."},"likely_asks":[{"question":"...","basis":"...","evidence":[...],"relevance":"high|medium|low","prepare":"..."}],"questions":[{"ask":"...","why":"...","listen_for":"...","evidence":[...]}],"scenarios":[{"label":"...","play":"...","evidence":[...]}]}.
Write every word from the briefing and from nothing else. Never invent a fact, a name, a date or a number. If the briefing does not say it, do not write it.
Quote what people actually asked for. A question that would read the same about any other company is worthless — name the thing this account said, in their words where the briefing has them.
Cite the ids the briefing gave you, in evidence only. An id must never appear in the text a reader sees.
Do not write the unknowns: the briefing lists what the record does not say, and that list is not yours to add to.
At most five likely asks, five questions and three scenarios. Three good questions beat five ordinary ones.
Never open with "Absolutely", "Great question", "I'd be happy to", "Based on the provided context", or any greeting. No exclamation marks. No praise.
Say plainly when something is uncertain rather than filling the gap. Omit a field you have nothing real for.
`

// planSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func planSystemFor(fence promptfence.Fence, lang string) string {
	return planSystem + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("meeting briefing")
}

// PlanPrompt is what the model is shown: enough to be specific, bounded enough
// to be affordable.
type PlanPrompt struct {
	Subject     string          `json:"subject"`
	StartsAt    time.Time       `json:"starts_at"`
	Company     string          `json:"company,omitempty"`
	MeetingType string          `json:"meeting_type"`
	TypeSignals []string        `json:"meeting_type_signals,omitempty"`
	Deal        *DealIn         `json:"deal,omitempty"`
	Project     *ProjectIn      `json:"project,omitempty"`
	Attendees   []AttendeeIn    `json:"attendees,omitempty"`
	Claims      []ClaimIn       `json:"captured_claims,omitempty"`
	Moments     []PromptMoment  `json:"account_moments,omitempty"`
	Unknowns    []PromptUnknown `json:"what_the_record_does_not_say,omitempty"`
	// MeetingID is the activity every recommendation may cite when it rests on
	// the meeting itself rather than on a conversation.
	MeetingID string `json:"meeting_id"`
}

// PromptMoment is one stretch of the account, with what was actually said in
// it. The excerpts are the point: without them the model has dates and
// subjects, and writes a plan that fits any account with dates and subjects.
type PromptMoment struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Title    string          `json:"title,omitempty"`
	Messages []PromptMessage `json:"messages,omitempty"`
}

// PromptMessage is one message, with the id a citation must name.
type PromptMessage struct {
	ActivityID string `json:"activity_id"`
	At         string `json:"at"`
	Direction  string `json:"direction,omitempty"`
	Subject    string `json:"subject,omitempty"`
	Text       string `json:"text,omitempty"`
}

// PromptUnknown is a gap, shown so the model can aim a question at it without
// being able to invent one.
type PromptUnknown struct {
	Kind     string `json:"kind"`
	Question string `json:"question"`
}

// planPromptOf projects the input and the deterministic floor into what the
// model reads.
func planPromptOf(in Input, floor Plan) PlanPrompt {
	prompt := PlanPrompt{
		Subject:     in.Subject,
		StartsAt:    in.StartsAt,
		Company:     in.Company,
		MeetingType: string(floor.Type.Value),
		TypeSignals: floor.Type.Signals,
		Deal:        in.Deal,
		Project:     in.Project,
		Attendees:   in.Attendees,
		Claims:      in.Commitments,
		MeetingID:   in.ActivityID,
	}
	byActivity := map[string]ExcerptIn{}
	for _, excerpt := range in.Excerpts {
		byActivity[excerpt.ActivityID] = excerpt
	}
	for _, moment := range floor.Arc {
		shown := PromptMoment{
			From:  moment.Moment.From.UTC().Format(time.DateOnly),
			To:    moment.Moment.To.UTC().Format(time.DateOnly),
			Title: moment.Moment.Title,
		}
		for _, current := range moment.Moment.Threads {
			for _, id := range current.IDs {
				excerpt, ok := byActivity[id]
				if !ok {
					continue
				}
				shown.Messages = append(shown.Messages, PromptMessage{
					ActivityID: excerpt.ActivityID,
					At:         excerpt.At.UTC().Format(time.DateOnly),
					Direction:  excerpt.Direction,
					Subject:    excerpt.Subject,
					Text:       excerpt.Text,
				})
			}
		}
		prompt.Moments = append(prompt.Moments, shown)
	}
	for _, unknown := range floor.Unknowns {
		prompt.Unknowns = append(prompt.Unknowns, PromptUnknown{
			Kind: string(unknown.Kind), Question: unknown.Question,
		})
	}
	return prompt
}

// PlanRequest builds the one request this site sends. Exported for the same
// reason BriefRequest is: a certification case must issue the request
// production issues, or it measures a copy that stays green through the change
// that breaks the original.
func PlanRequest(prompt PlanPrompt, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         planSystemFor(fence, lang),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encodePlanPrompt(prompt))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

func encodePlanPrompt(prompt PlanPrompt) string {
	encoded, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "encoding_error", err.Error())
	}
	return string(encoded)
}

// planNatureAllowed says what KIND of claim each plan field may carry.
//
// The plan is advice, so most of it is recommendation; the two fields that
// READ the account rather than propose a move are the risk and an ask's basis,
// and those are assessments. An arc summary states what happened, so it is a
// fact — a judgement there would be a reading dressed as a record.
var planNatureAllowed = map[string]map[string]bool{
	"objective":   {natureRecommendation: true},
	"opening":     {natureRecommendation: true},
	"top_risk":    {natureAssessment: true, natureFact: true},
	"likely_asks": {natureAssessment: true, natureFact: true},
	"advance":     {natureRecommendation: true},
	"account_arc": {natureFact: true},
}

// planNatureDefault is what a field's sentence is labelled when the model does
// not say. Spelled rather than taken from the allowed set, which is a map and
// would hand back a different answer per run for the fields that allow two.
var planNatureDefault = map[string]string{
	"objective":   natureRecommendation,
	"opening":     natureRecommendation,
	"top_risk":    natureAssessment,
	"likely_asks": natureAssessment,
	"advance":     natureRecommendation,
	"account_arc": natureFact,
}
