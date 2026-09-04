// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// Turning the assembled relationship into a brief — two ways, one of which
// always works.
//
// Deterministic first, because it is the floor: no model lane configured,
// budget exhausted, or a reply the validator refuses, and the reader still gets
// a brief. The model lane reads the same records and says what they add up to;
// it never adds one. Both paths cite the same records, so a sentence is
// checkable whichever wrote it, and `generated_by` names which of the two the
// reader is holding.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// The natures a sentence can carry, DERIVED from the contract's own enum rather
// than re-spelled, so a rename upstream fails to compile here instead of
// laundering a hand-typed string past the type. The person brief renders the
// company brief's sentence shape (doc.go: the mirroring is the ruling), so it
// is that enum for both.
const (
	natureFact           = string(crmcontracts.OrganizationBriefSentenceNatureFact)
	natureAssessment     = string(crmcontracts.OrganizationBriefSentenceNatureAssessment)
	natureRecommendation = string(crmcontracts.OrganizationBriefSentenceNatureRecommendation)
)

// knownNature is the vocabulary a reply may label a claim with. Anything else
// is read as a fact, which is the strictest reading: it must be grounded and it
// may not judge.
var knownNature = map[string]bool{
	natureFact: true, natureAssessment: true, natureRecommendation: true,
}

// maxRecommendations bounds the advice at ONE. The card is four or five
// sentences beside a page already full of actions; a brief offering three moves
// has handed the reader the triage it existed to do.
const maxRecommendations = 1

// maxSentences bounds the whole brief. Past this the card stops being a brief
// and becomes the record it was written to spare the reader.
const maxSentences = 5

// briefSystem is the person_brief site's prompt.
//
// Two rules carry the weight. Every claim is LABELLED with what kind of claim
// it is, so the reader can tell a stored fact from a reading of one — the whole
// difference between a brief they can check and a brief they must trust. And
// the brief is banned from restating transport: "you exchanged emails" is true
// of every contact in the system, and a card that says it has spent the
// reader's attention to tell them nothing.
const briefSystem = `You write the standing relationship brief on a contact's page, from a JSON summary of one person in a salesperson's CRM.
Return ONLY a JSON object: {"sentences":[{"text":"...","nature":"fact|assessment|recommendation","evidence":[{"entity_type":"person|deal|activity","entity_id":"..."}]}]}.
Answer, in order: what matters about this person NOW, what they have said they care about or object to, where the commercial stake stands, and — at most once — the single next move.
Lead with what CHANGED or what is outstanding. A brief that opens with the job title has buried its own finding.
Label every sentence. A FACT restates what the summary says and cites the record it came from. An ASSESSMENT is a judgment you draw by reading several records together — say it plainly, and cite the records that support it. A RECOMMENDATION is one concrete move; cite the record that motivates it. There is at most ONE recommendation.
Write about SUBSTANCE, never transport. "You exchanged emails", "they replied", "the last activity was a call" say nothing a reader could act on. Say what the conversation was about, in their own words where the summary quotes them.
Direction and answer state are the point: distinguish what they wrote to you from what you wrote to them, and an unanswered message from a settled one. The summary says which.
Name a date, an amount, a stage or a span only when the summary supplies it. Never compute one, never round one, and never estimate how long ago something was.
Never invent a fact. If the summary does not say it, you may still ASSESS it — but then it is an assessment and must be labelled one.
If the summary is thin, say what is MISSING and stop. Four honest sentences beat six padded ones, and a brief that pads is one a reader learns to skip.
Cite the ids the summary gave you. A sentence about the person themselves cites the person.
Put ids ONLY in evidence. An id must never appear in a sentence's text — the reader sees the text, and an id there is unreadable.
Write one claim per sentence, plainly, in the reader's second person where natural. Name the person once; after that they are "they".
If the summary names sections_omitted, say nothing about those subjects at all — the reader is not allowed to see them.`

// briefSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
//
// The brief takes the installation's language, like every other AI surface. It
// is cached per reader, but that split is about PERMISSIONS — the brief is
// assembled from records that reader may see, and two people with different
// access get different facts — not about preference. Language is not a
// permission, so it does not follow the reader.
func briefSystemFor(fence promptfence.Fence, lang string) string {
	return briefSystem + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("relationship summary")
}

// BriefRequest builds the one request this site sends. Exported because the
// certification case issues the SAME request production does — a case that
// rebuilt it would measure a copy, and a copy stays green through the change
// that breaks the original.
//
// The summary carries message subjects, message previews and the verbatim
// quotes behind each claim — text written by people outside this workspace. It
// is fenced with a nonce that writer has never seen, so no subject line or
// quoted sentence can close the span and be read as instruction.
func BriefRequest(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         briefSystemFor(fence, lang),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encodeInput(in))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

// encodeInput renders the assembled relationship as the JSON the prompt reads.
//
// A summary that cannot be encoded is a programming error, not a runtime one:
// Input is our own struct of scalars and slices. An empty prompt still reaches
// the model fenced, and the grounding filter refuses the reply that comes back.
func encodeInput(in Input) string {
	encoded, _ := json.Marshal(in) //nolint:errchkjson // Input is a plain struct of scalars; marshal cannot fail
	return string(encoded)
}

// Write produces the brief. lane may be nil, which is not an error state: it is
// the deployment saying this role runs no model, and the deterministic floor is
// the answer.
func Write(
	ctx context.Context, lane Completer, personID string, in Input, lang string,
) ([]Sentence, crmcontracts.WrittenBy, error) {
	floor := Deterministic(personID, in)
	if lane == nil {
		return floor, crmcontracts.Deterministic, nil
	}
	written, err := writeWithModel(ctx, lane, personID, in, lang)
	if err != nil {
		// The declared degrade posture, not a swallowed error. A model that is
		// unavailable, over budget, or answering unparseable JSON must not take
		// the card down with it: the reader gets the floor, and generated_by
		// tells them which of the two they are reading.
		//nolint:nilerr // on_budget_exhausted: degrade — the fallback IS the answer, and generated_by reports it
		return floor, crmcontracts.Deterministic, nil
	}
	return written, crmcontracts.Model, nil
}

func writeWithModel(
	ctx context.Context, lane Completer, personID string, in Input, lang string,
) ([]Sentence, error) {
	req := BriefRequest(in, lang)
	resp, err := ai.Ask(ctx, lane, req, func(text string) error {
		_, err := ParseBrief(text, personID, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	kept, err := ParseBrief(resp.Text, personID, in)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 {
		return nil, errors.New("the brief reply cited nothing about this person")
	}
	return kept, nil
}

// ParseBrief reads a model reply into grounded sentences. Exported for the same
// reason as BriefRequest: the certification case must run the filter production
// runs, because that filter is what stands between a reader and a sentence
// about a record they cannot open.
//
// personID pins the one contact this brief is about, so a person citation
// cannot name a different one.
func ParseBrief(reply, personID string, in Input) ([]Sentence, error) {
	var parsed struct {
		Sentences []Sentence `json:"sentences"`
	}
	// ai.Unfence, not a bare TrimSpace: a model that wraps its JSON in a ```json
	// fence is answering correctly, and every other model-reply parser in the
	// tree reduces through the same helper. Trimming whitespace alone drops the
	// whole model lane to the deterministic floor on those providers.
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return nil, fmt.Errorf("parse the brief reply: %w", err)
	}
	return keepGroundedSentences(parsed.Sentences, personID, in), nil
}

// keepGroundedSentences drops any sentence whose citations do not point at
// records this input actually carried, then applies the brief's two volume
// bounds.
//
// The reader's trust in the brief is the citation: a sentence pointing at an id
// that was never in the input is either invented or points somewhere the reader
// cannot go, and neither is worth showing. Dropping the sentence is the honest
// response — the remaining ones still say true things.
//
// Grounding decides FIRST, before the recommendation budget is spent. Counting
// an ungrounded recommendation would let one malformed claim suppress the valid
// advice behind it, and the reader would lose the advice and be told nothing
// about why.
func keepGroundedSentences(sentences []Sentence, personID string, in Input) []Sentence {
	known := knownRecords(personID, in)
	out := make([]Sentence, 0, min(len(sentences), maxSentences))
	recommendations := 0
	for _, sentence := range sentences {
		if len(out) == maxSentences {
			break
		}
		if !knownNature[sentence.Nature] {
			sentence.Nature = natureFact
		}
		if !claims.Grounded(sentence, known) {
			continue
		}
		if sentence.Nature == natureRecommendation {
			if recommendations == maxRecommendations {
				continue
			}
			recommendations++
		}
		out = append(out, sentence)
	}
	return claims.Dedupe(out)
}

// Prose is the brief's sentences as one readable block. Exported for the
// certification case, which asks whether the model's words say anything this
// relationship's own records produced — a question about the prose rather than
// about any one sentence's citations.
func Prose(sentences []Sentence) string {
	texts := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		texts = append(texts, sentence.Text)
	}
	return strings.Join(texts, " ")
}

// knownRecords is what this brief was written from, keyed by TYPE AND ID.
//
// Keying on the id alone would accept a real deal id cited as a person: the id
// passes, and the card then routes the reader to the wrong screen — or to a
// record of a kind they were never shown. The pair is the reference, so the
// pair is what is checked.
//
// A timeline row, a claim's source and a moment's evidence all name ACTIVITIES,
// and the same activity legitimately reaches this map by more than one of those
// routes.
func knownRecords(personID string, in Input) map[Evidence]bool {
	known := map[Evidence]bool{{EntityType: citePerson, EntityID: personID}: true}
	if in.OpenDeal != nil {
		known[Evidence{EntityType: citeDeal, EntityID: in.OpenDeal.ID}] = true
	}
	for _, act := range in.Recent {
		known[Evidence{EntityType: citeActivity, EntityID: act.ID}] = true
	}
	for _, claim := range in.Claims {
		known[Evidence{EntityType: citeActivity, EntityID: claim.SourceID}] = true
	}
	if in.Moment != nil {
		for _, source := range in.Moment.Sources {
			known[Evidence{EntityType: citeActivity, EntityID: source}] = true
		}
	}
	return known
}
