// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The model lane: the card's words.
//
// The deterministic composition in write.go stays the floor and the fallback,
// and it writes strictly less: the story and the blocker, from records.
//
// This lane writes the briefing — what happened, what is holding it up, what
// the buyer is after, whether the deal is real, and the line to open with. The
// three it adds are the three a fold over records cannot do: reading a motive
// out of somebody's own words, judging whether a deal will close, and putting
// a sentence in the reader's mouth.
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

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the deal_health lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// statusSystem is this site's prompt.
//
// The card is a BRIEFING, not a summary. The page beside it already shows the
// stage, the value and the timeline; a card that restates them has told the
// reader nothing they could not see. What it owes them is the reading: why the
// deal sits where it does, what the buyer is actually after, whether it is
// still alive, and what to say next.
//
// The instruction to write like a colleague rather than a report is doing real
// work here. A model asked for "a status" produces "engagement has been
// limited and next steps remain to be defined", which is a sentence about
// nothing. Asked to say what it would tell a colleague in the corridor, it
// says "she asked for times on 2 June and nobody sent them".
const statusSystem = `You brief a colleague on one sales deal, from a JSON summary of the deal, its timeline, its open tasks and its buyer conversation in a CRM.

Return ONLY a JSON object with these keys:
{"story":[...],"blocker":[...],"buyer":[...],"verdict":{"standing":"...","because":[...]},"move_reason":[...]}
Each of "story", "blocker", "buyer", "verdict.because" and "move_reason" is a list of {"text":"...","evidence":["<id>", ...]}.

"story" — what happened and where it leaves things, in the order it happened. Two to four sentences. Start with the thing a reader who has forgotten this deal most needs to know. Name people, dates and what was actually said.
"blocker" — what is HOLDING THE DEAL UP, named as something somebody can act on: an unsent mail, a question nobody answered, a person who never replied, a decision nobody has asked for. One or two sentences. Return an empty list when nothing is holding it up. "Time has passed" is not a blocker; "she asked for times on 2 June and nobody sent them" is.
"buyer" — what the buyer wants, read from what they have actually said: what they are optimising for, what they asked for, what they have NOT objected to. One or two sentences. Return an empty list when they have said too little to read honestly. Never guess at a motive the summary does not support.
"verdict" — your honest call. "standing" is exactly one of: live (moving, with a next step both sides expect), drifting (nothing wrong, nothing happening, it dies of neglect if nobody acts), blocked (something specific is in the way, and you named it in "blocker"), cold (a long silence after real engagement — treat as lost unless something changes). "because" is a LIST of one or two {"text","evidence"} objects saying what the call rests on — the same shape as "story", never a bare string. Be willing to say a deal is cold. A briefing that never delivers bad news is not read twice.
"move_reason" — a LIST of exactly one {"text","evidence"} object saying why the recommended move is the right one now, the same shape as "story". The move itself is decided elsewhere and given to you in "recommended_move": explain it, never replace it. It rests on records like every other sentence, so it cites them.

Every sentence lists the ids it rests on in its own "evidence", from the summary's "id" fields. Ids belong in "evidence" only — never in any "text" or in "opening".
Ground every word in the summary. Never invent a person, a company, a date, a number or an event. If the summary does not say it, do not write it.
Every timeline entry carries "when": "past" for something that has happened, "scheduled" for something booked and still ahead. A scheduled entry is a plan, never an event — never write that it took place, and never measure silence from it.
"open_tasks" is work NOBODY HAS DONE YET, whatever its date says. A task there carries "state": "open" or "overdue". Never write that a task's work happened, was sent, was followed up or was delivered — an overdue task is a promise already broken, not a thing that took place, and it is the strongest reason to act rather than evidence that somebody already did. Completed work is on the timeline instead, as the event it became.
"health" scores four things from 0 to 1, where low is bad: activity_recency, stage_velocity, engagement (how many people are actually talking to us) and commitments (promises we have kept). They are signals to reason from, never facts to state — never write a score, a factor name or the word "health" in the card. A low score tells you where to look in "timeline"; the timeline's dates are what you write.
Never write the same fact in two sections. Each one answers a different question.
`

// statusSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
// The card is filed on the deal and read by whoever opens it, so it takes the
// installation's shared language rather than the language the buyer's mail
// happened to be in — which is what an unruled prompt would have followed.
func statusSystemFor(fence promptfence.Fence, lang string) string {
	return statusSystem + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("deal timeline and buyer conversation")
}

// The reply's bounds. A card past these is a document, and the page already
// links the records it would be paraphrasing.
//
// The sentence bound is generous on purpose. A real status sentence carries a
// subject, a date and what it means — "They asked to move to 60-day payment
// terms on the 11th and we said we would come back this week, which has not
// happened" is 130 characters and entirely reasonable. A bound tight enough to
// feel neat rejects good cards, and the reader gets the deterministic one
// without ever learning why.
const (
	maxSentenceLen = 400
	maxMoveReason  = 300
	maxStoryRows   = 4
	maxBlockerRows = 2
	maxBuyerRows   = 2
	maxBecauseRows = 2
	maxOpeningLen  = 400
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
	// ReplyTo is the mail an answer is owed on, and it rides here for the
	// FINGERPRINT rather than for the prompt. It is read from the whole
	// timeline window while this summary carries only the newest rows, so a
	// deal with enough non-mail rows above the mail could otherwise change
	// which message is owed an answer without changing the key — and the card
	// would keep saying "Send an email" while somebody waits.
	ReplyTo string `json:"reply_to,omitempty"`
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

// FactorIn is one health factor as a MEASUREMENT: what was counted, and how
// low it scored. Deliberately no sentence.
//
// The formula's own explanatory prose ("no two-way contact in the last 90
// days") is written for a numeric readout, and handing it to a writer produces
// a card that quotes a statistic where the reader wanted the situation — and
// that reads the measurement WINDOW as elapsed time. A number and a label give
// the model the same signal with nothing to paste.
type FactorIn struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	// Count is what the factor counted, where counting is what it does:
	// engaged stakeholders, overdue tasks. Absent for the two that measure
	// time rather than things.
	Count *int `json:"count,omitempty"`
}

// ActIn is one timeline row, dated rather than aged, so the same facts build
// the same request and the router's result cache can serve a repeat view.
type ActIn struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Direction string `json:"direction,omitempty"`
	Subject   string `json:"subject,omitempty"`
	At        string `json:"at"`
	// When says whether this row has happened yet. A deal's timeline holds
	// BOTH — a booked meeting sits in it beside last week's mail — and a date
	// alone does not tell a writer which, so a card would report a meeting
	// scheduled for Thursday as one that already took place.
	When    string `json:"when"`
	Excerpt string `json:"excerpt,omitempty"`
}

// TaskIn is one open task: what is owed and by when.
type TaskIn struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Due     string `json:"due,omitempty"`
	// State is "open" or "overdue" — never "done", because a done task is not
	// in this list at all.
	//
	// Said rather than left to be worked out from `due` against today: the
	// model has no clock, and "overdue" is the whole difference between a
	// promise still in hand and one already broken.
	State string `json:"state"`
}

// The two states a task in this list can be in. A third would mean a task that
// is not open, which belongs on the timeline as the event it became.
//
// Exported because the certification fixture builds this same input, and a
// case that spelled the state itself could grade a value production never
// sends.
const (
	TaskStateOpen    = "open"
	TaskStateOverdue = "overdue"
)

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
func StatusRequest(in StatusInput, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         statusSystemFor(fence, lang),
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
	Story      []WrittenLine
	Blocker    []WrittenLine
	Buyer      []WrittenLine
	Verdict    WrittenVerdict
	MoveReason WrittenLine
}

// WrittenVerdict is the call and what it rests on. An empty Standing means the
// reply named no call this build recognises, and the card shows none rather
// than inventing one.
type WrittenVerdict struct {
	Standing string
	Because  []WrittenLine
}

// The calls a verdict may make. A reply naming anything else is refused: a
// card is allowed to say a deal is dead, but only in words the reader has
// learned to read.
var verdictStandings = map[string]bool{
	"live": true, "drifting": true, "blocked": true, "cold": true,
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
	var parsed replyShape
	if err := json.Unmarshal([]byte(reply), &parsed); err != nil {
		return WrittenStatus{}, fmt.Errorf("parse deal status: %w", err)
	}
	known, citable := knownIDs(in), citableIDs(in)
	sections, err := keepSections(parsed, known, citable)
	if err != nil {
		return WrittenStatus{}, err
	}
	// The story is the one section a briefing cannot do without: without it
	// the card opens with what is wrong about a deal it never described.
	if len(sections.Story) == 0 {
		return WrittenStatus{}, errors.New("deal status reply tells no story")
	}
	if err := keepMoveReason(&sections, parsed, known, citable); err != nil {
		return WrittenStatus{}, err
	}
	return sections, nil
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
