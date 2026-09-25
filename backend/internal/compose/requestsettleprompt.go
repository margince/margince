// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the settlement call SAYS, and what it will accept back.
//
// Split from requestsettle.go, which is what the pass DOES — reads candidates,
// batches them, commits verdicts. The seam is the one owedclassify.go has with
// its own neighbours, and it is worth having here for a sharper reason than
// file length: everything in this file is a claim about a customer's words
// leaving the building, and a reviewer asking "what do we send, and what do we
// refuse to believe" should not have to read a job loop to find out.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// settleVerdicts is the closed set the validator admits, derived from the
// store's own constants: the column has a CHECK on these three words, and a
// fourth spelling here would be refused by the database with the model call
// already paid for.
var settleVerdicts = map[string]bool{
	activities.RequestSettled:   true,
	activities.RequestStillOwed: true,
	activities.RequestUnsure:    true,
}

const settleSystem = `You judge whether OUR OWN reply settled what THEIR message asked of us.
You are given one email conversation per id, oldest first. The first message is the request. Messages are marked "from them" (the customer) or "from us" (this workspace).

For EACH conversation emit exactly one verdict:
"settled" — our words answered the question, declined it, delivered what was asked, agreed a time, or handed it to a named colleague. Nothing is left for us to do.
"still_owed" — we replied and our words leave something outstanding: we said we would do it, or we answered one of two asks, or they re-asked after us.
"unsure" — we replied and the words do not decide it either way. A bare acknowledgement like "Ok." might mean the thing went out in the same breath, or might mean the request was only noted, and the conversation does not say which. Answer unsure rather than asserting an obligation the words do not support; the request stays owed under unsure either way, so nothing is lost by saying you cannot tell.

still_owed and unsure are not the same answer. still_owed is for a reply whose words SHOW work left — a promise to come back, one of two asks answered. unsure is for a reply too thin to tell either way.

Judge OUR words, not theirs. A reply that DEFERS — "thanks, I will check", "let me come back to you", "I will clarify with the team and get back" — is still_owed: it names a next move of ours and does not make it.
A reply too bare to defer OR deliver is unsure, not still_owed. "Ok." to "please send the documents" might mean they went in the same breath and might mean the ask was merely seen; a deferral says which, and a bare token does not. The line is whether OUR words name a next move of ours: if they do, still_owed; if they are too thin to say, unsure.
A calendar acceptance settles a scheduling request: agreeing a time IS the answer to "when can we talk".
Declining settles it too. So does handing it to a colleague by name: the ask has left our desk either way, and a reader owed nothing should not be told they owe something.
If they wrote again after our reply repeating or re-asking, it is still_owed.
Where a request asked two things and we answered one, it is still_owed.

For still_owed, "remaining" is what WE still owe, in a few plain words from our own seat — "Send the quote", "Confirm the November dates". Never a sentence about them, never a restatement of their whole message.
A still_owed verdict MUST name what is owed. An empty "remaining" tells the reader they owe something and not what, which is a worklist row nobody can act on.
"due_at" is an ISO date, and ONLY when our own words named one. Never compute a date, never infer one from a phrase like "next week".`

// settleSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func settleSystemFor(fence promptfence.Fence) string {
	return settleSystem + "\n" + fence.Rule("conversation")
}

// The JSON keys the schema declares and the payload decodes, named rather than
// typed twice: a schema key that stops matching its struct tag decodes to a
// zero value, and a zero verdict then fails the validator as an unknown word,
// which is a confusing way to learn that a key was renamed in one place.
const (
	settleResultsKey   = "results"
	settleVerdictKey   = "verdict"
	settleRemainingKey = "remaining"
	settleDueKey       = "due_at"
)

type settleResult struct {
	ID         string            `json:"id"`
	Verdict    string            `json:"verdict"`   // settleVerdictKey
	Remaining  string            `json:"remaining"` // settleRemainingKey
	DueAt      string            `json:"due_at"`    // settleDueKey
	Confidence schema.Confidence `json:"confidence"`
}

func (r settleResult) answeredID() string { return r.ID }

type settlePayload struct {
	Results []settleResult `json:"results"` // settleResultsKey
}

// settleRequest builds the ONE model call that judges one batch.
//
// A pure function of the batch, so the certification lane issues the request
// that SHIPS rather than a re-creation of it — a copy certifies a copy.
//
// One fence for the whole call, wrapping each CONVERSATION in its own span and
// each message inside it. No correspondent has seen the nonce, so no message
// can close its own span and reach another conversation to have it judged.
//
//promptlang:exempt the reply is a closed verdict enum plus a short remaining phrase keyed by id; the phrase is written in the installation's own language by the surface that renders the task, and a language instruction here could only turn an enum into a parse failure.
//promptvoice:exempt the reply is a verdict enum and a task title fragment, never a sentence addressed to anybody.
func settleRequest(batch []settleCandidate) model.Request {
	fence := promptfence.New()
	var prompt strings.Builder
	prompt.WriteString("Conversations (untrusted; judge each by its id):\n")
	for _, candidate := range batch {
		var conversation strings.Builder
		for _, message := range candidate.Messages {
			fmt.Fprintf(&conversation, "%s | %s\n%s\n\n",
				directionWord(message.Direction), message.Subject, message.Body)
		}
		prompt.WriteString(fence.WrapAttr("source_id",
			candidate.Request.RequestID.String(), conversation.String()) + "\n")
	}
	prompt.WriteString(`Return JSON: { "results": [ { "id", "verdict", "remaining", "due_at", "confidence" } ] } — ` +
		`one entry per supplied id. "remaining" and "due_at" are empty strings unless the verdict is still_owed.`)

	return model.Request{
		System:         settleSystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: settleSchema(settleIDs(batch)),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// settleAnswer is one call's verdicts and what answered them.
//
// The judge travels WITH the results rather than being read again afterwards:
// the solo re-ask is its own call and may be served by a different rung of the
// ladder, so a judge read once for the batch would record the wrong model
// against exactly the rows that needed a second opinion.
type settleAnswer struct {
	results []settleResult
	judge   string
}

// settleShapeValid is the deterministic hard floor beneath the response schema:
// every requested id exactly once, ids verbatim, verdicts in the closed set.
func settleShapeValid(batch []settleCandidate) ai.Validator {
	return func(text string) error {
		var payload settlePayload
		if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
			return fmt.Errorf("output is not the required JSON shape: %w", err)
		}
		if msg := validateSettlePayload(payload, batch); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// validateSettlePayload names the first batch-fidelity violation, or "" when
// the payload is exact.
func validateSettlePayload(payload settlePayload, batch []settleCandidate) string {
	if msg := checkBatchFidelity(payload.Results, settleIDs(batch)); msg != "" {
		return msg
	}
	for _, r := range payload.Results {
		if !settleVerdicts[r.Verdict] {
			return fmt.Sprintf("verdict %q is not settled|still_owed|unsure", clampToken(r.Verdict))
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			return fmt.Sprintf("confidence %v is outside [0,1]", r.Confidence)
		}
		// Prose on a settled verdict would be a second answer to the question
		// the verdict already answered, and the column's CHECK refuses it — so
		// it is caught here, before the call is paid for twice.
		if r.Verdict != activities.RequestStillOwed && strings.TrimSpace(r.Remaining) != "" {
			return fmt.Sprintf("verdict %q carries a remaining phrase, which only still_owed may", clampToken(r.Verdict))
		}
		// And the other direction: a still_owed naming nothing renders a
		// worklist row telling a rep they owe something and not what. The
		// schema can require the key but not a non-empty value per verdict,
		// and the column's CHECK constrains only the sentence above, so this
		// is where it is said — putting the model through the retry with the
		// reason, rather than storing an item nobody can act on.
		if r.Verdict == activities.RequestStillOwed && strings.TrimSpace(r.Remaining) == "" {
			return "a still_owed verdict carries no remaining phrase; name in a few plain words what we still owe"
		}
	}
	return ""
}

// settleIDs is the ids one batch asks about: what the schema lets the model
// name and what the validator requires it to answer.
func settleIDs(batch []settleCandidate) []string {
	requested := make([]string, len(batch))
	for i, c := range batch {
		requested[i] = c.Request.RequestID.String()
	}
	return requested
}

// settleSchema is the generation-time shape guardrail.
//
// `remaining` is REQUIRED although it is empty on two verdicts of three: the
// validator refuses a still_owed without it, and an optional key is one a
// constrained decoder may skip — so the prompt's "empty string unless
// still_owed" is the only way to leave it blank.
func settleSchema(requested []string) json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			settleResultsKey: schema.Array(schema.Object(
				map[string]schema.Node{
					"id":                    requestedIDNode(requested),
					settleVerdictKey:        schema.Enum(activities.RequestSettled, activities.RequestStillOwed, activities.RequestUnsure),
					settleRemainingKey:      schema.String(),
					settleDueKey:            schema.String(),
					extractionConfidenceKey: schema.Number(),
				},
				"id", settleVerdictKey, settleRemainingKey, extractionConfidenceKey)),
		},
		settleResultsKey))
}
