// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How an OpenAI-wire reply ends. A broker reports everything after its 200 in
// the body — a refusal, a filter, an upstream that died mid-answer — so the
// status line says nothing and the choice has to be read for it.

import (
	"context"
	"fmt"
)

// openAICompatChoice is one choice as this wire returns it.
type openAICompatChoice struct {
	// FinishReason is the normalized stop reason; NativeFinishReason is the
	// upstream's own spelling of it, which a broker passes through and which is
	// the only place a withheld answer says which filter fired.
	FinishReason       string `json:"finish_reason"`
	NativeFinishReason string `json:"native_finish_reason"`
	// Error is a broker's report that the upstream failed AFTER the 200 went
	// out. Only the message is decoded: the code is a number on one broker and
	// a string on another, and a mistyped field would fail the whole decode.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message struct {
		Content string `json:"content"`
		// Refusal is the model's own refusal text, carried beside a null
		// Content rather than in it.
		Refusal string `json:"refusal"`
		// Reasoning is a reasoning model's thinking text, which this wire
		// carries BESIDE Content rather than inside it. It matters to a
		// non-reasoning caller for one reason: when the output budget is
		// spent before the answer begins, Content arrives null and all the
		// generated tokens are here. Reading Content alone then returns
		// empty text for a call that was billed in full.
		Reasoning string `json:"reasoning"`
	} `json:"message"`
}

// compatFinishError and compatContentFilter are this wire's two terminals that
// deliver no answer: the upstream failed, or a filter stopped it.
const (
	compatFinishError   = "error"
	compatContentFilter = "content_filter"
)

// terminal is the choice's ending in the port's vocabulary: stop and length
// come back as the finish reason (this wire already spells a cut-off reply
// model.FinishReasonLength), a refusal or filter as model.ErrOutputWithheld, and
// an upstream failure as an ordinary provider error the ladder may walk past.
func (c openAICompatChoice) terminal(ctx context.Context) (string, error) {
	if c.Error != nil || c.FinishReason == compatFinishError {
		detail := "no detail given"
		if c.Error != nil && c.Error.Message != "" {
			detail = safeProviderText(ctx, c.Error.Message)
		}
		return "", fmt.Errorf("ai: openai-compat: the upstream failed mid-answer: %s", detail)
	}
	if c.Message.Refusal != "" || c.FinishReason == compatContentFilter {
		reason := c.NativeFinishReason
		switch {
		case reason != "":
		case c.Message.Refusal != "":
			reason = finishRefusal
		default:
			reason = compatContentFilter
		}
		return "", withheldError{wire: "openai-compat", reason: safeProviderText(ctx, reason), detail: safeProviderText(ctx, c.Message.Refusal)}
	}
	return c.FinishReason, nil
}
