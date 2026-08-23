// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package nextaction

// The model lane: the fallback task, made concrete.
//
// The deterministic rules stay the authority on WHICH verb the card offers. A
// booked meeting, an unanswered inbound mail and an existing open task each
// answer without a model, and this lane never runs for them. It runs only on
// the fallback — the arm whose task the rules can only title "Agree the next
// step" — and its whole job is to replace that shrug with a task worth
// creating: a subject naming one concrete move, and a reason saying why that
// move, grounded in the deal's own timeline and in nothing else.
//
// No lane wired, a lane over budget, or a reply the filter refuses, and the
// reader gets the deterministic fallback exactly as before. `generated_by`
// says which of the two they are reading.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/ai"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/promptfence"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the deal_health lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// nextMoveSystem is this site's prompt. The reply is one task, not advice: the
// subject is what lands on the rep's list when they click, so it has to be an
// imperative a colleague could act on without opening the card again.
const nextMoveSystem = `You propose the single next step on one sales deal, from a JSON summary of the deal and its timeline in a CRM.
Return ONLY a JSON object: {"subject":"...","reason":"...","evidence":["<activity id>", ...]}.
"subject" is the task: one imperative sentence naming a concrete move — who to contact, what to send, what to clarify — at most 90 characters, no trailing period.
"reason" is why this move and why now: at most two short sentences drawn from the summary.
"evidence" lists the ids of the timeline entries the proposal rests on, from the summary's "id" fields. Ids belong in "evidence" only — an id must never appear in "subject" or "reason".
Ground every word in the summary. Never invent a person, a company, a date, a number or an event. If the summary does not say it, do not write it.
Prefer the move the timeline itself points at: an offer discussed but never sent, a question left hanging, a person who went quiet, a stage the deal has outsat.
If the timeline is empty, propose the honest opening move for THIS deal from its own fields (its name, value, stage, close date) — not a generic checklist item.
Voice: a calm, capable colleague. Lead with the move. No greetings, no praise, no exclamation marks, no "I recommend".`

// nextMoveSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func nextMoveSystemFor(fence promptfence.Fence) string {
	return nextMoveSystem + "\n" + fence.Rule("deal timeline")
}

// The reply's bounds. A subject past the cap is a paragraph pretending to be a
// task; a reason past it is a brief, and the card already links one.
const (
	maxSubjectLen = 120
	maxReasonLen  = 320
	// maxExcerptLen bounds what one timeline row contributes to the prompt, so
	// a pasted contract in a mail body does not become the whole context.
	maxExcerptLen = 400
	// maxTimelineRows bounds the summary. The newest rows carry the state of
	// play; a deal whose signal sits 30 rows deep is not a fallback-arm deal.
	maxTimelineRows = 12
)

// Input is the prompt's projection of the facts: the deal's own fields and
// the timeline rows the CALLER may read. It is assembled from facts gathered
// under the caller's row scope, so nothing reaches the model that the reader
// could not open themselves; a row whose content is withheld contributes its
// existence and date but no words.
type Input struct {
	Deal     DealIn  `json:"deal"`
	Timeline []ActIn `json:"timeline"`
}

// DealIn is the deal as the prompt sees it.
type DealIn struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Stage         string `json:"stage,omitempty"`
	Amount        string `json:"amount,omitempty"`
	ExpectedClose string `json:"expected_close,omitempty"`
}

// ActIn is one timeline row as the prompt sees it: dated, not aged, so the
// same facts build the same request and the router's result cache can serve a
// repeat view instead of paying for it again.
type ActIn struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Direction string `json:"direction,omitempty"`
	Subject   string `json:"subject,omitempty"`
	At        string `json:"at"`
	Excerpt   string `json:"excerpt,omitempty"`
}

// NextMoveInput projects the gathered facts into the prompt's shape.
func NextMoveInput(f facts) Input {
	in := Input{Deal: dealIn(f.deal)}
	for _, a := range f.timeline {
		if len(in.Timeline) == maxTimelineRows {
			break
		}
		in.Timeline = append(in.Timeline, actIn(a))
	}
	return in
}

func dealIn(d crmcontracts.Deal) DealIn {
	out := DealIn{ID: d.Id.String(), Name: d.Name, Status: string(d.Status)}
	if d.AmountMinor != nil && d.Currency != nil && *d.Currency != "" {
		out.Amount = fmt.Sprintf("%d %s (minor units)", *d.AmountMinor, *d.Currency)
	}
	if d.ExpectedCloseDate != nil {
		out.ExpectedClose = d.ExpectedCloseDate.Format("2006-01-02")
	}
	return out
}

func actIn(a crmcontracts.Activity) ActIn {
	out := ActIn{ID: a.Id.String(), Kind: string(a.Kind), At: a.OccurredAt.Format("2006-01-02")}
	if a.Direction != nil {
		out.Direction = string(*a.Direction)
	}
	// A withheld row keeps its place in the story — contact happened on this
	// date — and gives up its words: the reader may not open them, so the
	// model may not read them.
	if withheld(a) {
		return out
	}
	if a.Subject != nil {
		out.Subject = *a.Subject
	}
	if a.Body != nil {
		out.Excerpt = excerpt(*a.Body)
	}
	return out
}

// excerpt bounds one body's contribution, cutting on a rune so a multi-byte
// character is never split into an invalid tail.
func excerpt(body string) string {
	trimmed := strings.TrimSpace(body)
	runes := []rune(trimmed)
	if len(runes) <= maxExcerptLen {
		return trimmed
	}
	return string(runes[:maxExcerptLen]) + "…"
}

// writeNextMove asks the lane for the concrete task and folds it over the
// deterministic fallback. The verb, the links and the source stay the floor's:
// the model writes WHAT the task says, never what clicking it does.
func writeNextMove(ctx context.Context, lane Completer, f facts, floor crmcontracts.DealNextBestAction) (crmcontracts.DealNextBestAction, error) {
	in := NextMoveInput(f)
	resp, err := lane.Complete(ctx, NextMoveRequest(in))
	if err != nil {
		return crmcontracts.DealNextBestAction{}, err
	}
	move, err := ParseNextMove(resp.Text, in)
	if err != nil {
		return crmcontracts.DealNextBestAction{}, err
	}
	out := floor
	args := map[string]any{}
	for k, v := range *floor.Arguments {
		args[k] = v
	}
	args["subject"] = move.Subject
	out.Arguments = &args
	out.Reason = move.Reason
	out.Evidence = append([]crmcontracts.DealNextBestActionEvidence{}, floor.Evidence...)
	for _, cited := range move.Evidence {
		if row, ok := timelineRow(f, cited); ok {
			out.Evidence = append(out.Evidence, evidenceOf(row, "Grounds: "+subjectOf(row)))
		}
	}
	return out, nil
}

func timelineRow(f facts, id string) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if a.Id.String() == id {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}

// NextMoveRequest builds the one request this site sends. Exported because the
// certification case must issue the SAME request production does — a case that
// rebuilt it would measure a copy, and a copy stays green through the change
// that breaks the original.
//
// The summary carries mail subjects and body excerpts — text written by people
// outside this workspace. It is fenced with a nonce the writer has never seen,
// so no subject line can close the span and be read as instruction.
func NextMoveRequest(in Input) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         nextMoveSystemFor(fence),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encodeInput(in))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

// encodeInput renders the assembled facts as the JSON the prompt reads. Every
// field is a plain value this package built, so a marshal failure is a
// programming error; it is still surfaced as text rather than dropped, so a
// malformed request fails the model call loudly.
func encodeInput(in Input) string {
	encoded, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "encoding_error", err.Error())
	}
	return string(encoded)
}

// NextMove is what the lane proposes once the filter has kept it.
type NextMove struct {
	Subject  string
	Reason   string
	Evidence []string
}

// ParseNextMove reads the reply, keeping it only when it is grounded and
// within bounds. Exported for the same reason as NextMoveRequest: the
// certification case must run the filter production runs.
func ParseNextMove(reply string, in Input) (NextMove, error) {
	var parsed struct {
		Subject  string   `json:"subject"`
		Reason   string   `json:"reason"`
		Evidence []string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(reply), &parsed); err != nil {
		return NextMove{}, fmt.Errorf("parse next move: %w", err)
	}
	subject := strings.TrimSpace(parsed.Subject)
	reason := strings.TrimSpace(parsed.Reason)
	if subject == "" || reason == "" {
		return NextMove{}, errors.New("next move reply carries no task")
	}
	if len([]rune(subject)) > maxSubjectLen || len([]rune(reason)) > maxReasonLen {
		return NextMove{}, errors.New("next move reply exceeds the card's bounds")
	}
	known := knownIDs(in)
	// An id in reader-visible text is either a leak or filler; either way the
	// reply is not the one the prompt asked for.
	for id := range known {
		if strings.Contains(subject, id) || strings.Contains(reason, id) {
			return NextMove{}, errors.New("next move reply spells a record id in reader text")
		}
	}
	move := NextMove{Subject: subject, Reason: reason}
	// A citation pointing outside the input is invented; the ones that remain
	// still say where the proposal came from. When the timeline was empty the
	// deal's own fields are the grounding, and no citation is owed.
	for _, cited := range parsed.Evidence {
		if known[cited] && cited != in.Deal.ID {
			move.Evidence = append(move.Evidence, cited)
		}
	}
	if len(in.Timeline) > 0 && len(move.Evidence) == 0 {
		return NextMove{}, errors.New("next move reply cites nothing on the timeline it was written from")
	}
	return move, nil
}

// knownIDs is every id this input carried — the set a citation must come from
// and reader text must be free of.
func knownIDs(in Input) map[string]bool {
	known := map[string]bool{in.Deal.ID: true}
	for _, a := range in.Timeline {
		known[a.ID] = true
	}
	return known
}
