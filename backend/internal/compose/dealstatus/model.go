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

	"github.com/gradionhq/margince/backend/internal/compose/promptlang"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/ai"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/promptfence"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
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
{"story":[...],"blocker":[...],"buyer":[...],"verdict":{"standing":"...","because":[...]},"move_reason":"..."}
Each of "story", "blocker", "buyer" and "verdict.because" is a list of {"text":"...","evidence":["<id>", ...]}.

"story" — what happened and where it leaves things, in the order it happened. Two to four sentences. Start with the thing a reader who has forgotten this deal most needs to know. Name people, dates and what was actually said.
"blocker" — what is HOLDING THE DEAL UP, named as something somebody can act on: an unsent mail, a question nobody answered, a person who never replied, a decision nobody has asked for. One or two sentences. Return an empty list when nothing is holding it up. "Time has passed" is not a blocker; "she asked for times on 2 June and nobody sent them" is.
"buyer" — what the buyer wants, read from what they have actually said: what they are optimising for, what they asked for, what they have NOT objected to. One or two sentences. Return an empty list when they have said too little to read honestly. Never guess at a motive the summary does not support.
"verdict" — your honest call. "standing" is exactly one of: live (moving, with a next step both sides expect), drifting (nothing wrong, nothing happening, it dies of neglect if nobody acts), blocked (something specific is in the way, and you named it in "blocker"), cold (a long silence after real engagement — treat as lost unless something changes). "because" is one or two sentences saying what the call rests on. Be willing to say a deal is cold. A briefing that never delivers bad news is not read twice.
"move_reason" — one sentence on why the recommended move is the right one now. The move itself is decided elsewhere and given to you in "recommended_move": explain it, never replace it.

Every sentence in "story", "blocker", "buyer" and "verdict.because" lists the ids it rests on in its own "evidence", from the summary's "id" fields. Ids belong in "evidence" only — never in any "text", in "move_reason" or in "opening".
Ground every word in the summary. Never invent a person, a company, a date, a number or an event. If the summary does not say it, do not write it.
Every timeline entry carries "when": "past" for something that has happened, "scheduled" for something booked and still ahead. A scheduled entry is a plan, never an event — never write that it took place, and never measure silence from it.
"health" scores four things from 0 to 1, where low is bad: activity_recency, stage_velocity, engagement (how many people are actually talking to us) and commitments (promises we have kept). They are signals to reason from, never facts to state — never write a score, a factor name or the word "health" in the card. A low score tells you where to look in "timeline"; the timeline's dates are what you write.
Never write the same fact in two sections. Each one answers a different question.

Voice: a capable colleague briefing you in the corridor, in plain words. Say "she asked for times and nobody sent them", not "follow-up communication remains outstanding". Short sentences. No corporate register, no hedging, no "it appears that", no greetings, no praise, no exclamation marks, no restating the deal's name.`

// statusSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
// The card is filed on the deal and read by whoever opens it, so it takes the
// installation's shared language rather than the language the buyer's mail
// happened to be in — which is what an unruled prompt would have followed.
func statusSystemFor(fence promptfence.Fence, lang string) string {
	return statusSystem + "\n" + promptlang.Rule(lang) + "\n" + fence.Rule("deal timeline and buyer conversation")
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
	MoveReason string
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
	if err := keepFreeText(&sections, parsed, known); err != nil {
		return WrittenStatus{}, err
	}
	return sections, nil
}

// replyShape is the reply as the prompt asks for it.
type replyShape struct {
	Story   []replyLine `json:"story"`
	Blocker []replyLine `json:"blocker"`
	Buyer   []replyLine `json:"buyer"`
	Verdict struct {
		Standing string      `json:"standing"`
		Because  []replyLine `json:"because"`
	} `json:"verdict"`
	MoveReason string `json:"move_reason"`
}

// keepSections runs the grounding filter over every cited section. Each may
// come back empty except the story, and an empty one means the records did not
// support saying anything — which is a section the card omits rather than pads.
func keepSections(parsed replyShape, known, citable map[string]bool) (WrittenStatus, error) {
	var out WrittenStatus
	for _, section := range []struct {
		name  string
		lines []replyLine
		limit int
		into  *[]WrittenLine
	}{
		{"story", parsed.Story, maxStoryRows, &out.Story},
		{"blocker", parsed.Blocker, maxBlockerRows, &out.Blocker},
		{"buyer", parsed.Buyer, maxBuyerRows, &out.Buyer},
		{"verdict", parsed.Verdict.Because, maxBecauseRows, &out.Verdict.Because},
	} {
		kept, err := keepGrounded(section.lines, known, citable, section.limit)
		if err != nil {
			return WrittenStatus{}, fmt.Errorf("%s: %w", section.name, err)
		}
		*section.into = kept
	}
	// A call this build does not recognise is dropped rather than shown: the
	// reader has learned what four words mean, and a fifth teaches them
	// nothing. The reasoning behind it stays, because it is still grounded.
	if verdictStandings[parsed.Verdict.Standing] {
		out.Verdict.Standing = parsed.Verdict.Standing
	}
	return out, nil
}

// keepFreeText holds the one field that carries no citations of its own: the
// move's reason.
//
// It is uncited deliberately. The reason explains a move the RULES chose from
// records, so what it rests on is the move's own evidence rather than a
// citation of its own. It is still bounded and still refused when it spells an
// id.
func keepFreeText(out *WrittenStatus, parsed replyShape, known map[string]bool) error {
	reason := strings.TrimSpace(parsed.MoveReason)
	if len([]rune(reason)) > maxMoveReason {
		return errors.New("deal status reply's move reason exceeds the card's bounds")
	}
	if err := refuseIDsInReaderText(reason, known); err != nil {
		return fmt.Errorf("move reason: %w", err)
	}
	out.MoveReason = reason
	return nil
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
func keepGrounded(
	lines []replyLine, known, citable map[string]bool, limit int,
) ([]WrittenLine, error) {
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
			// The deal's own id grounds nothing — every sentence is about this
			// deal — and it is absent from the citable set for that reason.
			if citable[id] {
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

// knownIDs is every id this input carried — the set reader text must be free
// of. Being known is not the same as being CITABLE: see citableIDs.
func knownIDs(in StatusInput) map[string]bool {
	known := citableIDs(in)
	known[in.Deal.ID] = true
	if in.Room != nil {
		for _, th := range in.Room.Threads {
			known[th.ID] = true
		}
	}
	return known
}

// citableIDs is the narrower set a citation may come from: the records the
// card can render as evidence the reader opens.
//
// A Deal Room thread is deliberately absent. The card cites through the
// activity evidence type, and a thread is not an activity — admitting one
// would pass the filter and then be dropped when the card is assembled, which
// costs the sentence its grounding after it had already earned it. The buyer's
// words still reach the model; they are just cited through the deal's timeline
// rather than by thread id.
func citableIDs(in StatusInput) map[string]bool {
	citable := map[string]bool{}
	for _, a := range in.Timeline {
		citable[a.ID] = true
	}
	for _, t := range in.OpenTasks {
		citable[t.ID] = true
	}
	return citable
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
