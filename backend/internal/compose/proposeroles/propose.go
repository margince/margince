// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package proposeroles reads a deal's buying roles out of what its contacts
// have actually written.
//
// The roles are the deal's shape: who signs, who carries it inside the account,
// who can stop it. They are recorded by hand today, which means a deal usually
// has none — and a committee with no champion recorded looks exactly like a
// committee with no champion.
//
// A ROLE IS NEVER INFERRED FROM A JOB TITLE. The contract says so where the
// field is declared, and it is the whole reason this reads messages rather than
// signatures: "Managing Director" says what somebody is called, not that they
// carry this deal. A proposal citing only a title is dropped.
//
// WRITTEN DIRECTLY, NOT STAGED. The installation's posture is that an agent
// writes and the writing is marked, attributed and reversible, rather than
// queueing for a human who then rubber-stamps it. That puts the whole weight on
// the gate below: nothing reaches a record unless the model quoted a real
// message, that quote is genuinely in the source it named, and the seat it
// proposes is one nobody has filled by hand.
package proposeroles

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// ConfidenceFloor is the score a proposal must clear to be written.
//
// Higher than the enrichment floor (0.6) on purpose. A wrong phone number is a
// typo a reader corrects in passing; a wrong economic buyer sends a rep to the
// wrong person for a quarter, and the page above it then reports the committee
// as complete. The cost of being wrong is what sets the bar, not the model.
const ConfidenceFloor = 0.75

// Candidate is one contact this read may propose a role for, with the messages
// it is allowed to read them from.
type Candidate struct {
	PersonID string
	FullName string
	Title    string
	// HoldsRole says a person has already answered this question for this
	// contact. The read may not overwrite them.
	HoldsRole bool
	// Messages the contact themselves wrote, newest first. Their own words are
	// the only evidence that says what they do on this deal.
	Messages []Message
}

// Message is one thing a contact wrote, and the id that proves where it came
// from.
type Message struct {
	ActivityID string
	Subject    string
	Body       string
}

// Proposal is one role the model read out of the evidence.
type Proposal struct {
	PersonID        string  `json:"person_id"`
	Role            string  `json:"role"`
	EvidenceSnippet string  `json:"evidence_snippet"`
	SourceID        string  `json:"source_id"`
	Confidence      float64 `json:"confidence"`
}

const systemPrompt = `You read buying roles out of messages a customer's own people wrote.

The roles: champion (carries the deal inside their company), economic_buyer
(signs for it), blocker (can stop it), influencer (shapes the decision without
making it), user (lives with what is bought).

Rules you must not break:
- Quote the message you read it from, verbatim, in evidence_snippet. Copy the
  words exactly as they appear; do not paraphrase, translate or tidy them.
- Name that message's source_id.
- A JOB TITLE IS NOT EVIDENCE. "Managing Director" says what somebody is
  called, not what they do on this deal. Read what they WROTE.
- Propose nothing you are unsure of. A deal with no evidence of a role yields
  no proposal for it, which is the correct answer and not a failure.
- One proposal per person at most.`

// Request builds the model call.
//
// The fence is minted per call: a boundary reused across calls is one a
// previous sender has already been shown, and every message here is written by
// somebody outside this company.
//
//promptlang:exempt the payload is the customers' own messages, each carrying an evidence_snippet checked verbatim against the source it names — translating one would both change the words and fail that check.
//promptvoice:exempt the payload is other people's messages under a fence; there is no sentence of ours here to have a voice.
func Request(dealName string, candidates []Candidate) model.Request {
	fence := promptfence.New()
	var prompt strings.Builder
	// The deal's NAME is record data — somebody typed it, and on a shared deal
	// that somebody may not be us. Interpolated into the instruction region it
	// would be read in the prompt's own voice, which is the whole attack, so it
	// is fenced like everything else that came from a person.
	prompt.WriteString("Deal (untrusted):\n")
	prompt.WriteString(fence.Wrap(dealName) + "\n\n")
	for _, candidate := range candidates {
		prompt.WriteString("Contact (untrusted):\n")
		// The name and title go INSIDE the boundary with everything else they
		// wrote. A title interpolated into a header line would be read in the
		// prompt's own voice, which is exactly the attack — and the rule above
		// says a title is not evidence anyway.
		prompt.WriteString(
			fence.Wrap(fmt.Sprintf("person_id: %s\nName: %s\nTitle: %s",
				candidate.PersonID, candidate.FullName, candidate.Title)) + "\n")
		for _, message := range candidate.Messages {
			prompt.WriteString("They wrote (untrusted):\n")
			prompt.WriteString(fence.WrapAttr("source_id", message.ActivityID,
				message.Subject+"\n"+message.Body) + "\n")
		}
		prompt.WriteString("\n")
	}
	prompt.WriteString(`Return JSON: { "proposals": [ { "person_id", "role", "evidence_snippet", "source_id", "confidence" } ] }`)

	return model.Request{
		System:         systemPrompt + "\n\n" + fence.Rule("untrusted material"),
		Messages:       []model.Message{{Role: "user", Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: Schema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// Schema is the shape the model must answer in.
func Schema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"proposals": schema.Array(schema.Object(
				map[string]schema.Node{
					"person_id":        schema.String(),
					"role":             schema.Enum(dealrole.Shown...),
					"evidence_snippet": schema.String(),
					"source_id":        schema.String(),
					"confidence":       schema.Number(),
				},
				"person_id", "role", "evidence_snippet", "source_id", "confidence",
			)),
		},
		"proposals",
	))
}

// MinEvidenceWords is how many words a quote must carry to be evidence.
//
// A substring check alone is not one: "I" occurs in almost every message, so a
// one-word snippet satisfies the letter of "quote your source" while supporting
// nothing. Six words is short enough for a real sentence fragment and long
// enough that it cannot be found by accident.
const MinEvidenceWords = 6

// Gate is what stands between a model's answer and a customer's record.
//
// Nothing here trusts the model. Every proposal has to survive four separate
// checks, and each one exists because failing it would put a false fact on a
// deal that a rep would then act on:
//
//   - The snippet must appear VERBATIM in the message the proposal names. A
//     quote the source does not contain is a sentence the model wrote, and a
//     record citing it would look checked while citing nothing.
//   - The source must be one this call actually supplied. A source_id from
//     somewhere else cannot be checked at all.
//   - The confidence must clear the floor.
//   - The person must be a candidate, and hold no role yet. A seat somebody
//     typed by hand is a human's answer, and overwriting it with a guess is the
//     one thing this must never do.
//
// A proposal that fails any of them is dropped, silently and without a retry:
// the honest answer to weak evidence is no answer.
func Gate(proposals []Proposal, candidates []Candidate) []Proposal {
	// Every source is bound to the person who WROTE it. Keyed by activity id
	// alone, a proposal could cite one contact's message as evidence about
	// another — and since both sit in the same prompt, a sender who writes an
	// instruction into their own email could hand a role to a colleague they
	// have never spoken for. The pair is what makes evidence evidence.
	sources := map[string]struct {
		author string
		body   string
	}{}
	known := map[string]bool{}
	held := map[string]bool{}
	for _, candidate := range candidates {
		known[candidate.PersonID] = true
		if candidate.HoldsRole {
			held[candidate.PersonID] = true
		}
		for _, message := range candidate.Messages {
			sources[message.ActivityID] = struct {
				author string
				body   string
			}{
				author: candidate.PersonID,
				body:   message.Subject + "\n" + message.Body,
			}
		}
	}
	roles := map[string]bool{}
	for _, role := range dealrole.Shown {
		roles[role] = true
	}

	seen := map[string]bool{}
	kept := make([]Proposal, 0, len(proposals))
	for _, proposal := range proposals {
		if seen[proposal.PersonID] {
			continue
		}
		if !survives(proposal, known, held, roles, sources) {
			continue
		}
		seen[proposal.PersonID] = true
		kept = append(kept, proposal)
	}
	return kept
}

// survives asks the four questions of one proposal.
//
// Its own function because each answer is a separate reason a record would be
// wrong, and a reader checking one of them should not have to hold the loop
// that walks them.
func survives(
	proposal Proposal,
	known, held, roles map[string]bool,
	sources map[string]struct {
		author string
		body   string
	},
) bool {
	if !known[proposal.PersonID] {
		return false
	}
	// A seat somebody typed is a human's answer. Overwriting it with a reading
	// is the one thing this must never do.
	if held[proposal.PersonID] {
		return false
	}
	// Outside [0,1] it is not a confidence at all: a model answering 75 for
	// "75%" would clear a 0.75 floor by a hundredfold and defeat it.
	if proposal.Confidence < ConfidenceFloor || proposal.Confidence > 1 {
		return false
	}
	if !roles[proposal.Role] {
		return false
	}
	// The evidence has to be the PERSON'S OWN words. Keyed by activity alone, a
	// sender who writes an instruction into their own email could hand a role to
	// a colleague they have never spoken for.
	src, ok := sources[proposal.SourceID]
	if !ok || src.author != proposal.PersonID {
		return false
	}
	snippet := strings.TrimSpace(proposal.EvidenceSnippet)
	if len(strings.Fields(snippet)) < MinEvidenceWords {
		return false
	}
	return strings.Contains(src.body, snippet)
}
