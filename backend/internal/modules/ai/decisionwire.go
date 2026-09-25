// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The decision wire: a structured state and typed questions in, a calibrated
// answer per question out. OpenRouter's decisions endpoint (Jev) and a
// same-host Laya encoder answer the same shape at different paths, so one
// client serves both and the path comes from the provider's registry row.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// errDecisionRejected marks a decision request the wire refused as malformed:
// a spec bug on this side, which the router logs at error level. It still
// falls back to the ladder, which asks the same question in its own form.
var errDecisionRejected = errors.New("ai: the decision model rejected the request")

// maxDecisionResponseBytes bounds one decoded answer. A choice answer is a few
// hundred bytes per question; anything near this is not an answer.
const maxDecisionResponseBytes = 1 << 20

type decisionWireQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type decisionWireRequest struct {
	Model     string                          `json:"model"`
	State     json.RawMessage                 `json:"state"`
	Questions map[string]decisionWireQuestion `json:"questions"`
}

type decisionWireAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// decisionWireResponse is the fields read off an answer. Jev names the served
// snapshot in `model` and the serving vendor in `provider` (observed
// 2026-09-25); Laya echoes the checkpoint and names no provider, and adds
// fields of its own that nothing here reads.
type decisionWireResponse struct {
	Model    string                        `json:"model"`
	Provider string                        `json:"provider"`
	Answers  map[string]decisionWireAnswer `json:"answers"`
	Usage    struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

// decisionClient sends decision requests to one endpoint.
type decisionClient struct {
	http *http.Client
	// url is the binding's base_url joined with the provider's decision path.
	url string
	// apiKey is the key owner's credential; empty for a keyless local adapter.
	apiKey string
}

func (c *decisionClient) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	body, err := json.Marshal(decisionToWire(req))
	if err != nil {
		return decision.Response{}, fmt.Errorf("ai: decision: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return decision.Response{}, fmt.Errorf("ai: decision: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := sendModelRequest(c.http, httpReq)
	if err != nil {
		return decision.Response{}, fmt.Errorf("ai: decision: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close of a response body already read — the decode result or the status error is the answer
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return decision.Response{}, decisionError(ctx, resp)
	}
	var wire decisionWireResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxDecisionResponseBytes)).Decode(&wire); err != nil {
		return decision.Response{}, fmt.Errorf("ai: decision: unreadable response: %w", err)
	}
	return decisionFromWire(wire), nil
}

func decisionToWire(req decision.Request) decisionWireRequest {
	questions := make(map[string]decisionWireQuestion, len(req.Questions))
	for name, q := range req.Questions {
		questions[name] = decisionWireQuestion{Type: string(q.Type), Instructions: q.Instructions, Criteria: q.Criteria}
	}
	return decisionWireRequest{Model: req.Model, State: req.State, Questions: questions}
}

func decisionFromWire(wire decisionWireResponse) decision.Response {
	answers := make(map[string]decision.Answer, len(wire.Answers))
	for name, a := range wire.Answers {
		answers[name] = decision.Answer{Choice: a.Choice, Confidence: a.Confidence, Probabilities: a.Probabilities}
	}
	return decision.Response{
		Answers: answers, InputTokens: wire.Usage.InputTokens,
		ServedModel: wire.Model, ServedProvider: wire.Provider,
	}
}

// decisionError says only what the wire's structured message says, redacted
// and capped, never the raw body. A 429 is classified as every adapter's is,
// so the fallback reads a refusal the same way; a 400 or 422 is a request this
// side built wrong.
func decisionError(ctx context.Context, resp *http.Response) error {
	err := providerRefusal(resp, "", fmt.Errorf("ai: decision: %s (http %d)", decisionErrorDetail(ctx, resp), resp.StatusCode))
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
		return fmt.Errorf("%w: %w", errDecisionRejected, rejectedRequest(err))
	}
	return err
}

// decisionErrorDetail reads the two structured shapes the decision wires
// answer with — a broker's {"error":{"message"}} and a plain {"message"} — and
// falls back to a fixed phrase for anything else, since an unstructured body
// may be HTML that echoes the request.
func decisionErrorDetail(ctx context.Context, resp *http.Response) string {
	var body struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil || json.Unmarshal(raw, &body) != nil {
		return "the decision endpoint refused the call"
	}
	for _, message := range []string{body.Error.Message, body.Message} {
		if detail := safeProviderText(ctx, message); detail != "" {
			return detail
		}
	}
	return "the decision endpoint refused the call"
}
