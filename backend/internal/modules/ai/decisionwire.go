// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The decision wire: a structured state and typed questions in, a calibrated
// answer per question out. TypeSafe's own API, a broker, a self-hosted server
// and Google's Decisions API answer the same shape. So one client serves them
// all, posting to the binding's endpoint as written. Google spells the request
// and its key differently, and the dialect says which.

import (
	"bytes"
	"cmp"
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

// geminiDecisionRequest is Google's spelling: the state goes as `input` text,
// which the API reads as the content the questions are about.
type geminiDecisionRequest struct {
	Model     string                          `json:"model"`
	Input     string                          `json:"input"`
	Questions map[string]decisionWireQuestion `json:"questions"`
}

// decisionDialect is which spelling of the decision wire an adapter speaks.
type decisionDialect uint8

const (
	// dialectJev sends `state` as JSON under a bearer key.
	dialectJev decisionDialect = iota
	// dialectGemini sends `input` text under Google's x-goog-api-key header.
	dialectGemini
)

type decisionWireAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// decisionWireResponse is the fields read off an answer. A hosted endpoint
// names the served snapshot in `model` and the serving vendor in `provider`.
// A self-hosted server may echo the checkpoint, name no provider, and add
// fields nothing here reads. Google names neither, and counts input as
// `total_input_tokens` where the Jev wire says `input_tokens`.
type decisionWireResponse struct {
	Model    string                        `json:"model"`
	Provider string                        `json:"provider"`
	Answers  map[string]decisionWireAnswer `json:"answers"`
	Usage    struct {
		InputTokens      int `json:"input_tokens"`
		TotalInputTokens int `json:"total_input_tokens"`
	} `json:"usage"`
}

// decisionClient sends decision requests to one endpoint.
type decisionClient struct {
	http *http.Client
	// url is the full decision endpoint, posted to as written.
	url string
	// apiKey is the adapter's credential; empty when it holds none, and then
	// no key header is sent.
	apiKey  string
	dialect decisionDialect
}

func (c *decisionClient) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	body, err := c.encode(req)
	if err != nil {
		return decision.Response{}, fmt.Errorf("ai: decision: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return decision.Response{}, fmt.Errorf("ai: decision: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.authorize(httpReq)
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

// encode spells req in the client's dialect.
func (c *decisionClient) encode(req decision.Request) ([]byte, error) {
	questions := wireQuestions(req)
	if c.dialect == dialectGemini {
		return json.Marshal(geminiDecisionRequest{Model: req.Model, Input: string(req.State), Questions: questions})
	}
	return json.Marshal(decisionWireRequest{Model: req.Model, State: req.State, Questions: questions})
}

func wireQuestions(req decision.Request) map[string]decisionWireQuestion {
	questions := make(map[string]decisionWireQuestion, len(req.Questions))
	for name, q := range req.Questions {
		questions[name] = decisionWireQuestion{Type: string(q.Type), Instructions: q.Instructions, Criteria: q.Criteria}
	}
	return questions
}

// authorize signs a request with the held key in the dialect's header. Google's
// header is one Go keeps across a redirect, which newOutboundClient refuses
// off-host, and getListBody refuses outright.
func (c *decisionClient) authorize(r *http.Request) {
	switch {
	case c.apiKey == "":
	case c.dialect == dialectJev:
		r.Header.Set("Authorization", "Bearer "+c.apiKey)
	case c.dialect == dialectGemini:
		r.Header.Set("x-goog-api-key", c.apiKey)
	}
}

func decisionFromWire(wire decisionWireResponse) decision.Response {
	answers := make(map[string]decision.Answer, len(wire.Answers))
	for name, a := range wire.Answers {
		answers[name] = decision.Answer{Choice: a.Choice, Confidence: a.Confidence, Probabilities: a.Probabilities}
	}
	return decision.Response{
		Answers: answers, InputTokens: cmp.Or(wire.Usage.InputTokens, wire.Usage.TotalInputTokens),
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
