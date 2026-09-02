// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one model call that judges one thread, and the validation that decides
// whether its answer is believed at all.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// confidentialityResult is one model answer about one thread.
type confidentialityResult struct {
	ID         string            `json:"id"`
	Verdict    string            `json:"verdict"`
	Confidence schema.Confidence `json:"confidence"`
}

type confidentialityPayload struct {
	Results []confidentialityResult `json:"results"`
}

// confidentialityRequest builds the ONE call that judges ONE thread. A pure
// function of the ledger row, so the certification lane issues the request that
// ships rather than a re-created copy of it.
//
// The fence is minted per request: its scope is this one call's text, and a
// boundary reused across calls is one a previous thread has already been shown.
//
// ONE THREAD PER CALL, for the reason the sender engine judges one sender per
// call: the only text in the prompt is the text of the thread being judged, so
// a hostile message has nobody else to speak for. It cannot dictate another
// thread's answer, and an answer its own content breaks is charged to it alone.
//
//promptlang:exempt the reply is one kind enum value and a confidence number — validateConfidentialityPayload refuses any other token, so there is no sentence here for a language to apply to.
//promptvoice:exempt the reply is one kind enum value and a confidence number.
func confidentialityRequest(row capture.PendingThread) model.Request {
	fence := promptfence.New()
	var prompt strings.Builder
	prompt.WriteString("Email thread (untrusted; judge it by its id):\n")
	// One span over the whole thread. The fields are concatenated INSIDE the
	// fence, so there is no seam between two separately wrapped spans for a
	// sender's text to close between them, and the boundary is the nonce rather
	// than any recognisable marker.
	//
	// Attachment names travel with the body because a name carries the answer
	// on its own: `Aufhebungsvertrag_final.pdf` is a termination agreement
	// whatever the covering note says.
	thread := fmt.Sprintf("Subject: %s\nAttachments: %s\n%s",
		row.Subject, strings.Join(row.Attachments, ", "), row.Body)
	prompt.WriteString(fence.WrapAttr("id", row.ID.String(), thread) + "\n")
	prompt.WriteString(`Return JSON: { "results": [ { "id", "verdict", "confidence" } ] } — one entry for the supplied id, where "verdict" is the thread kind.`)

	return model.Request{
		System:         confidentialitySystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: confidentialitySchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// confidentialitySchema is the generation-time shape guardrail. Its enum is
// derived from the taxonomy rather than hand-listed: on a grammar-constrained
// local rung the model cannot emit a kind this enum omits, whatever the prompt
// says, so a hand-written copy that fell behind would make a kind unreachable
// in production with every test still green.
func confidentialitySchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"results": schema.Array(schema.Object(
				map[string]schema.Node{
					"id":                    schema.String(),
					"verdict":               schema.Enum(confidentialityKindNames()...),
					extractionConfidenceKey: schema.Number(),
				},
				"id", "verdict", "confidence",
			)),
		},
		"results",
	))
}

// ask makes one structured call about one thread.
func (e *ConfidentialityVerdictEngine) ask(ctx context.Context, row capture.PendingThread) ([]confidentialityResult, error) {
	req := confidentialityRequest(row)
	validate := confidentialityShapeValid(row)
	var resp model.Response
	var err error
	if structured, ok := e.brain.(validatedBrain); ok {
		resp, err = structured.CompleteValidated(ctx, req, validate)
	} else {
		resp, err = e.brain.Complete(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	var payload confidentialityPayload
	if err := json.Unmarshal([]byte(ai.Unfence(resp.Text)), &payload); err != nil {
		return nil, fmt.Errorf("confidentiality: unparseable model output: %w", err)
	}
	if msg := validateConfidentialityPayload(payload, row); msg != "" {
		return nil, fmt.Errorf("confidentiality: %s", msg)
	}
	return payload.Results, nil
}

// confidentialityShapeValid is the generation-time validator: the requested id
// exactly once, verbatim, with a kind in the closed set and a confidence in
// range. An answer about an id nobody asked about is refused outright rather
// than partially believed.
func confidentialityShapeValid(row capture.PendingThread) ai.Validator {
	return func(text string) error {
		var payload confidentialityPayload
		if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
			return fmt.Errorf("output is not the required JSON shape: %w", err)
		}
		if msg := validateConfidentialityPayload(payload, row); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// validateConfidentialityPayload names the first fidelity violation, or "" when
// the payload is exact.
func validateConfidentialityPayload(payload confidentialityPayload, row capture.PendingThread) string {
	want := map[string]bool{row.ID.String(): true}
	seen := map[string]bool{}
	for _, r := range payload.Results {
		// Every echoed token is MODEL output, and a sender who got the model to
		// obey can choose it, so it is bounded before it reaches an error string
		// an operator will read from a log.
		if !want[r.ID] {
			return fmt.Sprintf("result id %q was not requested", clampToken(r.ID))
		}
		if seen[r.ID] {
			return fmt.Sprintf("result id %q appears twice", clampToken(r.ID))
		}
		seen[r.ID] = true
		if _, known := statusForConfidentiality(r.Verdict); !known {
			return fmt.Sprintf("kind %q is not one of %s",
				clampToken(r.Verdict), strings.Join(confidentialityKindNames(), "|"))
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			return fmt.Sprintf("confidence %v is outside [0,1]", r.Confidence)
		}
	}
	for id := range want {
		if !seen[id] {
			return fmt.Sprintf("no answer for thread %q", clampToken(id))
		}
	}
	return ""
}
