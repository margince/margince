// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The model lane: the card's words.
//
// The deterministic composition in write.go stays the floor and the fallback.
// This lane writes the same three parts better — reading the deal's timeline,
// its health factors, its open tasks and what the buyer said in the room, and
// saying where the deal stands, what could lose it, and the one move to make.
//
// The VERB never moves. Which action the card offers is decided by the rules
// in move.go from records, not by the model: a reader clicking "Draft the
// reply" must reach the mail the rules picked, not one the model imagined.
// The lane writes what the move SAYS; the rules decide what clicking it does.

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

// statusSystem is this site's prompt. The card is read at a glance before a
// call, so every sentence has to earn its line: no preamble, no restating the
// deal's name back at a reader who is looking at it.
const statusSystem = `You write the status of one sales deal, from a JSON summary of the deal, its timeline, its open tasks and its buyer conversation in a CRM.
Return ONLY a JSON object: {"standing":[{"text":"...","evidence":["<id>", ...]}, ...],"risk":[{"text":"...","evidence":["<id>", ...]}, ...],"move_reason":"..."}.
"standing" is where the deal is now: at most three sentences. What moved, what is waiting, what the buyer last said.
"risk" is what could lose this deal: at most two sentences. Write it ONLY when the summary shows something wrong — silence after a proposal, a close date the deal cannot make, a question nobody answered, an objection still open. When nothing is wrong, return an empty list. Never write a reassurance.
"move_reason" is one sentence saying why the recommended next move is the right one now, at most 200 characters. The move itself is decided elsewhere and given to you in "recommended_move" — explain it, never replace it.
Every sentence in "standing" and "risk" lists the ids it rests on in its own "evidence", from the summary's "id" fields. Ids belong in "evidence" only — an id must never appear in any "text" or in "move_reason".
Ground every word in the summary. Never invent a person, a company, a date, a number or an event. If the summary does not say it, do not write it.
Voice: a calm, capable colleague briefing you in the corridor. Lead with what matters. No greetings, no praise, no exclamation marks, no "I recommend", no restating the deal's name.`

// statusSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func statusSystemFor(fence promptfence.Fence) string {
	return statusSystem + "\n" + fence.Rule("deal timeline and buyer conversation")
}

// The reply's bounds. A card past these is a document, and the page already
// links the records it would be paraphrasing.
const (
	maxSentenceLen  = 240
	maxMoveReason   = 240
	maxStandingRows = 3
	maxRiskRows     = 2
	// maxExcerptLen bounds what one row contributes, so a pasted contract in
	// a mail body does not become the whole context.
	maxExcerptLen = 400
	// maxTimelineRows bounds the summary. The newest rows carry the state of
	// play.
	maxTimelineRows = 12
	// maxThreadRows bounds the buyer conversation the same way.
	maxThreadRows = 6
)

// StatusInput is the prompt's projection of the facts: only what the CALLER
// may read, because the facts were gathered under their row scope. A row whose
// content is withheld contributes its existence and date but no words.
type StatusInput struct {
	Deal      DealIn     `json:"deal"`
	Health    []FactorIn `json:"health,omitempty"`
	Timeline  []ActIn    `json:"timeline,omitempty"`
	OpenTasks []TaskIn   `json:"open_tasks,omitempty"`
	Room      *RoomIn    `json:"room,omitempty"`
	// RecommendedMove is the verb the rules chose. The model explains it and
	// never replaces it, which is why it rides in the summary rather than
	// being something the reply may set.
	RecommendedMove string `json:"recommended_move"`
}

// DealIn is the deal as the prompt sees it. No stage NAME: the facts carry
// only the stage's id, and a fixture supplying a name production never sends
// would certify a better-informed model than readers get.
type DealIn struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Amount        string `json:"amount,omitempty"`
	ExpectedClose string `json:"expected_close,omitempty"`
}

// FactorIn is one health factor: the number and the fact behind it, so the
// model can say WHY a deal is at risk rather than that a score is low.
type FactorIn struct {
	Key    string  `json:"key"`
	Value  float64 `json:"value"`
	Reason string  `json:"reason"`
}

// ActIn is one timeline row, dated rather than aged, so the same facts build
// the same request and the router's result cache can serve a repeat view.
type ActIn struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Direction string `json:"direction,omitempty"`
	Subject   string `json:"subject,omitempty"`
	At        string `json:"at"`
	Excerpt   string `json:"excerpt,omitempty"`
}

// TaskIn is one open task: what is owed and by when.
type TaskIn struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Due     string `json:"due,omitempty"`
}

// RoomIn is the Deal Room's state and what the buyer said in it.
type RoomIn struct {
	State   string     `json:"state"`
	Threads []ThreadIn `json:"threads,omitempty"`
}

// ThreadIn is one conversation, with whether the buyer flagged it as needing
// work — an open required-change thread is the clearest risk signal a room
// carries.
type ThreadIn struct {
	ID             string `json:"id"`
	Opener         string `json:"opener,omitempty"`
	State          string `json:"state"`
	RequiredChange bool   `json:"required_change,omitempty"`
}

// StatusRequest builds the one request this site sends. Exported because the
// certification case must issue the SAME request production does — a case that
// rebuilt it would measure a copy, and a copy stays green through the change
// that breaks the original.
//
// The summary carries mail subjects, body excerpts and buyer comments — text
// written by people outside this workspace. It is fenced with a nonce the
// writer has never seen, so no subject line can close the span and be read as
// instruction.
func StatusRequest(in StatusInput) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         statusSystemFor(fence),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encodeInput(in))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

// encodeInput renders the assembled facts as the JSON the prompt reads. Every
// field is a plain value this package built, so a marshal failure is a
// programming error; it is still surfaced as text rather than dropped, so a
// malformed request fails the model call loudly.
func encodeInput(in StatusInput) string {
	encoded, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "encoding_error", err.Error())
	}
	return string(encoded)
}

// WrittenStatus is what the lane proposes once the filter has kept it.
type WrittenStatus struct {
	Standing   []WrittenLine
	Risk       []WrittenLine
	MoveReason string
}

// WrittenLine is one sentence and the ids it rests on.
type WrittenLine struct {
	Text     string
	Evidence []string
}

// ParseStatus reads the reply, keeping it only when it is grounded and within
// bounds. Exported for the same reason as StatusRequest: the certification
// case must run the filter production runs.
//
// A refusal here is not a failure to handle — it is the declared degrade
// posture, and the caller composes the deterministic card instead.
func ParseStatus(reply string, in StatusInput) (WrittenStatus, error) {
	var parsed struct {
		Standing   []replyLine `json:"standing"`
		Risk       []replyLine `json:"risk"`
		MoveReason string      `json:"move_reason"`
	}
	if err := json.Unmarshal([]byte(reply), &parsed); err != nil {
		return WrittenStatus{}, fmt.Errorf("parse deal status: %w", err)
	}
	known := knownIDs(in)
	standing, err := keepGrounded(parsed.Standing, known, in.Deal.ID, maxStandingRows)
	if err != nil {
		return WrittenStatus{}, fmt.Errorf("standing: %w", err)
	}
	if len(standing) == 0 {
		return WrittenStatus{}, errors.New("deal status reply says nothing about where the deal stands")
	}
	// Risk is allowed to be empty and that is the point: a card that always
	// finds something wrong teaches the reader to stop reading the risk line.
	risk, err := keepGrounded(parsed.Risk, known, in.Deal.ID, maxRiskRows)
	if err != nil {
		return WrittenStatus{}, fmt.Errorf("risk: %w", err)
	}
	reason := strings.TrimSpace(parsed.MoveReason)
	if len([]rune(reason)) > maxMoveReason {
		return WrittenStatus{}, errors.New("deal status reply's move reason exceeds the card's bounds")
	}
	if err := refuseIDsInReaderText(reason, known); err != nil {
		return WrittenStatus{}, fmt.Errorf("move reason: %w", err)
	}
	return WrittenStatus{Standing: standing, Risk: risk, MoveReason: reason}, nil
}

// replyLine is one sentence as the reply spells it.
type replyLine struct {
	Text     string   `json:"text"`
	Evidence []string `json:"evidence"`
}

// keepGrounded drops a sentence whose citations do not resolve, and refuses
// the whole reply when a sentence breaks a bound. The difference matters: an
// ungrounded sentence is one bad claim among good ones, while an oversized or
// id-leaking one says the reply is not the shape the prompt asked for.
func keepGrounded(lines []replyLine, known map[string]bool, dealID string, limit int) ([]WrittenLine, error) {
	out := make([]WrittenLine, 0, len(lines))
	for _, line := range lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		if len([]rune(text)) > maxSentenceLen {
			return nil, errors.New("a sentence exceeds the card's bounds")
		}
		if err := refuseIDsInReaderText(text, known); err != nil {
			return nil, err
		}
		cited := make([]string, 0, len(line.Evidence))
		for _, id := range line.Evidence {
			// The deal's own id grounds nothing: every sentence is about this
			// deal, so citing it says only that the model read the prompt.
			if known[id] && id != dealID {
				cited = append(cited, id)
			}
		}
		// A sentence citing nothing is dropped whole rather than shown
		// uncited, which is the one thing the grounding rule exists to
		// prevent.
		if len(cited) == 0 {
			continue
		}
		out = append(out, WrittenLine{Text: text, Evidence: cited})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// refuseIDsInReaderText refuses a reply that spells a record id where a person
// reads. An id in reader text is either a leak or filler.
func refuseIDsInReaderText(text string, known map[string]bool) error {
	for id := range known {
		if strings.Contains(text, id) {
			return errors.New("reply spells a record id in reader text")
		}
	}
	return nil
}

// knownIDs is every id this input carried — the set a citation must come from
// and reader text must be free of.
func knownIDs(in StatusInput) map[string]bool {
	known := map[string]bool{in.Deal.ID: true}
	for _, a := range in.Timeline {
		known[a.ID] = true
	}
	for _, t := range in.OpenTasks {
		known[t.ID] = true
	}
	if in.Room != nil {
		for _, th := range in.Room.Threads {
			known[th.ID] = true
		}
	}
	return known
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

// dealIn projects the deal's own fields.
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
