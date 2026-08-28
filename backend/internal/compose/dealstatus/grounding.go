// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The grounding filter: what a reply has to prove before a word of it reaches
// the reader.
//
// One rule, applied to every sentence the card shows — cite a record this deal
// carries, or do not appear. It lives beside the request rather than inside it
// because the two answer different questions: model.go asks for prose, and this
// decides which of it is admissible.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// replyShape is the reply as the prompt asks for it.
type replyShape struct {
	Story   []replyLine `json:"story"`
	Blocker []replyLine `json:"blocker"`
	Buyer   []replyLine `json:"buyer"`
	Verdict struct {
		Standing string     `json:"standing"`
		Because  replyLines `json:"because"`
	} `json:"verdict"`
	MoveReason replyLines `json:"move_reason"`
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

// keepMoveReason holds the move's reason to the rule every other sentence on
// this card keeps: cite a record, or do not appear.
//
// It used to be the exception, on the reasoning that the reason explains a move
// the RULES chose, so what it rests on is the move's own evidence. That is true
// of the MOVE and not of the SENTENCE. What reached the reader was free prose
// bounded only by length and a no-ids check — the one field on the card a
// crafted mail body on the deal's timeline, or a model having a bad day, could
// fill with an invented rationale that still shipped as generated_by=model.
//
// An ungrounded reason is DROPPED rather than refused, and that is the
// difference between this and a section: writtenMove then keeps the
// deterministic reason the rules already produced, which is a true sentence
// about the same move. Refusing the whole reply would throw away a card that
// is otherwise entirely grounded.
//
// A reply that answers the older shape — a bare string, from a different
// provider or a cheaper lane — decodes through replyLines into one UNCITED
// line, which this filter drops. So the old shape degrades to the
// deterministic reason instead of passing the check it predates.
func keepMoveReason(out *WrittenStatus, parsed replyShape, known, citable map[string]bool) error {
	kept, err := keepGrounded(parsed.MoveReason, known, citable, 1)
	if err != nil {
		return fmt.Errorf("move reason: %w", err)
	}
	if len(kept) == 0 {
		// Nothing grounded, so nothing written: writtenMove keeps the
		// deterministic reason the rules already produced, which is a true
		// sentence about the same move rather than a blank line.
		return nil
	}
	out.MoveReason = kept[0].Text
	return nil
}

// replyLine is one sentence as the reply spells it.
type replyLine struct {
	Text     string   `json:"text"`
	Evidence []string `json:"evidence"`
}

// replyLines is a list of cited sentences that also accepts a BARE STRING.
//
// It has to, and the reason is worth stating because the card spent its whole
// life without a verdict on account of it. The prompt describes this field
// twice — once in the shape line as a list, once in prose as "one or two
// sentences" — and the model followed the prose, returning a string. The
// decoder refused it, the verdict was dropped, and the card fell back to the
// deterministic writer EVERY TIME, logging a warning nobody was reading. A
// reader saw a card with no call and no way to tell that one had been written.
//
// The prompt is fixed too, but a prompt is a request and this is the parse. A
// model can always answer the older shape — a different provider, a cheaper
// lane, a retry — and losing the whole verdict over a JSON shape is the wrong
// trade when the sentence itself is right there. A bare string becomes one
// uncited line: the grounding filter then treats it as any other uncited
// sentence, so nothing skips the check that matters.
type replyLines []replyLine

func (r *replyLines) UnmarshalJSON(raw []byte) error {
	var lines []replyLine
	if err := json.Unmarshal(raw, &lines); err == nil {
		*r = lines
		return nil
	}
	var bare string
	if err := json.Unmarshal(raw, &bare); err != nil {
		return fmt.Errorf("verdict.because is neither a list of sentences nor a string: %w", err)
	}
	if strings.TrimSpace(bare) == "" {
		*r = nil
		return nil
	}
	*r = replyLines{{Text: bare}}
	return nil
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
