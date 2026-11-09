// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the summarize lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// The natures a sentence can carry, derived from the contract's enum.
const (
	natureFact           = string(crmcontracts.DealRoomBriefSentenceNatureFact)
	natureAssessment     = string(crmcontracts.DealRoomBriefSentenceNatureAssessment)
	natureRecommendation = string(crmcontracts.DealRoomBriefSentenceNatureRecommendation)
)

var knownNature = map[string]bool{
	natureFact: true, natureAssessment: true, natureRecommendation: true,
}

// maxRecommendations is one: the tab already offers every verb, and a brief
// with three moves hands the reader back the triage it existed to do.
const maxRecommendations = 1

// addressInProse matches a link or a mail address. Buyers write the threads the
// brief reads, so a move to an address is a buyer's instruction.
var addressInProse = regexp.MustCompile(`(?i)(https?://|www\.)\S+|[^\s@]+@[^\s@]+\.[a-z]{2,}`)

// briefSystem is the deal_room_brief site's prompt. It leads with what a buyer
// is waiting on, a buyer's unmet condition for signing ahead of routine asks.
// It never attributes the reader's own side's words to a buyer.
const briefSystem = `You write the short brief at the top of a Deal Room, from a JSON summary of one room a salesperson shares with a buyer.
Return ONLY a JSON object: {"sentences":[{"text":"...","nature":"fact|assessment|recommendation","evidence":[{"entity_type":"participant|document|thread","entity_id":"<the record's ref, such as p1>"}]}]}.
Write at most four sentences. Answer, in order: what a buyer is waiting on the reader for, which buyer cannot get in or has not come, what the buyers have and have not read, and — at most once — the single next move.
Lead with a thread whose waiting_on_you is true. Read every such thread before choosing. When no buyer sets a condition, the first sentence names every waiting ask, the oldest included: it has waited longest. If a comment whose speaker is "buyer" says they will not sign, approve or go ahead until or without something, and no later comment met, withdrew or replaced that condition, the thread leads, whatever its date. The first sentence names the buyer in that comment's own author field, cites that comment's author_id as the participant, never an author of another comment or thread, and the term the condition is about, in words that stand without the thread. An ask that only coordinates access, attendance, scheduling or retrieval, and sets no condition, is routine and comes after it. required_change marks a thread opened as a change request on a document; it says how the thread began, not that the change is still owed. The order of the threads, and which is newest, never decides the lead. A ref's number (t1, t2) is a label, not a rank or a date.
A comment from "your_side" that sets a condition ("we cannot countersign until") is the reader's own position: never report it as what a buyer will not do or is waiting for. A buyer comment that only says they are reviewing and will come back asks nothing of the reader.
Say what the buyer ASKED, in their own words where the summary quotes them; "they left a comment" says nothing a reader could act on.
A comment's speaker is "buyer" or "your_side". "your_side" is the READER or a colleague. Never report words from "your_side" as a buyer's.
"downloaded" lists the documents a buyer downloaded. A document no buyer downloaded has not been read by anyone the room can see; say so only when it matters.
asked_for_new_link_on is when a buyer asked for a new link and has not been in since; they cannot get in until the reader issues one.
Name a date only when the summary supplies it. Never compute one, and never estimate how long ago something was.
Label every sentence. A FACT restates the summary and cites the record it came from. An ASSESSMENT is a judgment drawn from several records; cite them. A RECOMMENDATION is one concrete move; cite the record that motivates it.
Cite only the refs the summary gave you: a buyer as participant, a document as document, a thread as thread. Put refs ONLY in evidence, never in the text.
If nothing is waiting and nothing is missing, say so in one sentence and stop.`

func briefSystemFor(fence promptfence.Fence, lang string) string {
	return briefSystem + "\n" + promptvoice.Rule + "\n" + promptvoice.Move + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("room summary")
}

// BriefRequest builds the one request this site sends. Exported so the
// certification case issues the request production does. The summary carries
// text buyers wrote, so it is fenced with a nonce they have never seen.
func BriefRequest(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         briefSystemFor(fence, lang),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(shownRefs(in).Show(encodeInput(in)))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

func encodeInput(in Input) string {
	encoded, _ := json.Marshal(in) //nolint:errchkjson // Input is a plain struct of scalars; marshal cannot fail
	return string(encoded)
}

// shownRefs names each record id the room prompt shows by a ref, and reads a
// reply's citations back.
func shownRefs(in Input) claims.Refs {
	return claims.MintRefs(encodeInput(in), knownRecords(in))
}

// Write produces the brief. A nil lane is a deployment running no model, and
// the floor is its answer.
func Write(ctx context.Context, lane Completer, in Input, lang string) ([]Sentence, crmcontracts.WrittenBy) {
	floor := Deterministic(in, lang)
	if lane == nil || len(floor) == 0 {
		return floor, crmcontracts.WrittenByDeterministic
	}
	written, err := writeWithModel(ctx, lane, in, lang)
	if err != nil {
		// The declared degrade posture: the reader gets the floor, and
		// generated_by says so.
		return floor, crmcontracts.WrittenByDeterministic
	}
	return written, crmcontracts.WrittenByModel
}

func writeWithModel(ctx context.Context, lane Completer, in Input, lang string) ([]Sentence, error) {
	resp, err := ai.Ask(ctx, lane, BriefRequest(in, lang), func(text string) error {
		_, err := ParseBrief(text, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	kept, err := ParseBrief(resp.Text, in)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 {
		return nil, errors.New("the room brief reply cited nothing in this room")
	}
	return kept, nil
}

// ParseBrief reads a model reply into grounded sentences. Exported so the
// certification case runs the filter production runs.
func ParseBrief(reply string, in Input) ([]Sentence, error) {
	var parsed struct {
		Sentences []Sentence `json:"sentences"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return nil, fmt.Errorf("parse the room brief reply: %w", err)
	}
	return keepGroundedSentences(shownRefs(in).Read(parsed.Sentences), in), nil
}

// keepGroundedSentences applies the one grounding rule and this brief's bounds.
func keepGroundedSentences(sentences []Sentence, in Input) []Sentence {
	return claims.KeepBrief(sentences, knownRecords(in), claims.BriefBounds{
		Sentences:       maxSentences,
		Recommendations: maxRecommendations,
		Natures:         knownNature,
		Fact:            natureFact,
		Recommendation:  natureRecommendation,
		Refuse:          func(s Sentence) bool { return addressInProse.MatchString(s.Text) },
	})
}

// knownRecords is what the brief was written from, keyed by type and id, with
// what each record said as the model was shown it.
func knownRecords(in Input) map[Evidence]string {
	known := make(map[Evidence]string, len(in.Buyers)+len(in.Documents)+len(in.Threads))
	for _, buyer := range in.Buyers {
		known[Evidence{EntityType: citeParticipant, EntityID: buyer.ID}] = claims.Source(buyer)
	}
	for _, doc := range in.Documents {
		known[Evidence{EntityType: citeDocument, EntityID: doc.ID}] = claims.Source(doc)
	}
	for _, thread := range in.Threads {
		known[Evidence{EntityType: citeThread, EntityID: thread.ID}] = claims.Source(thread)
	}
	return known
}
