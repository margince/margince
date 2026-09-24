// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How a Gemini reply ends: the finishReason every candidate carries, read the
// same way by Complete and by the stream.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// geminiResponseError maps an in-body error object or a withholding
// finishReason to an error. Gemini delivers SAFETY, RECITATION, … inside a 200
// body, so unchecked they would pass for a complete answer. STOP and
// MAX_TOKENS are not errors: both carry the text generated, and absent is a
// non-final stream chunk.
func geminiResponseError(out geminiResponse) error {
	if out.Error.Message != "" || out.Error.Status != "" {
		return fmt.Errorf("ai: gemini: %s: %s", out.Error.Status, out.Error.Message)
	}
	// A blocked PROMPT has no candidate to carry a finishReason at all: the
	// reason is under promptFeedback, and without reading it the reply looks
	// like a body cut short in transit.
	if len(out.Candidates) == 0 && out.PromptFeedback.BlockReason != "" {
		return withheldError{wire: providerGemini, reason: "PROMPT_BLOCKED_" + out.PromptFeedback.BlockReason, detail: "the prompt was blocked"}
	}
	for _, cand := range out.Candidates {
		switch cand.FinishReason {
		case "", geminiStop, geminiMaxTokens:
		default:
			return stoppedError{reason: cand.FinishReason}
		}
	}
	return nil
}

// geminiStop and geminiMaxTokens are the two terminals that deliver an answer:
// whole, or cut off at maxOutputTokens.
const (
	geminiStop      = "STOP"
	geminiMaxTokens = "MAX_TOKENS"
	// geminiMissingThoughtSignature is the terminal for a multi-turn request
	// that dropped the signature a thinking model's earlier turn carried.
	geminiMissingThoughtSignature = "MISSING_THOUGHT_SIGNATURE"
)

// geminiFinishReason is Gemini's terminal in the port's vocabulary: a cut-off
// reply is model.FinishReasonLength on every wire, because the structured
// retry and the cert lane's ungraded run read that value and no other.
func geminiFinishReason(reason string) string {
	if reason == geminiMaxTokens {
		return model.FinishReasonLength
	}
	return reason
}

// stoppedError is a withholding finishReason as an error that still NAMES the
// terminal.
//
// Carrying it as data is what keeps the distinction recoverable: every
// withheld answer classifies to the one `provider_error` sentinel, so a refused
// one (SAFETY) and a recited one (RECITATION) are indistinguishable in a stored
// call unless the reason itself survives. A truncation is not one of them on
// Complete — it is a Response — and only a stream, whose port has no terminal
// to carry it, still ends on this error at MAX_TOKENS.
//
// The message is byte-identical to the plain error it replaces: callers and
// tests match on the text, so the reason is additive rather than a reword.
type stoppedError struct{ reason string }

func (e stoppedError) Error() string { return "ai: gemini: generation stopped: " + e.reason }

// FinishReason satisfies the accessor tracing.go probes for with errors.As,
// so the terminal reaches the trace without this package's error type
// leaking into the tracing path's imports.
func (e stoppedError) FinishReason() string { return e.reason }

// Unwrap classifies the terminal. Every reason but the two that deliver text is
// the provider's decision about this reply, so it is withheld; the exception is
// a thought signature the REQUEST failed to echo back, which is ours to fix and
// fails the same way on every retry.
func (e stoppedError) Unwrap() error {
	switch e.reason {
	case geminiMaxTokens:
		return nil
	case geminiMissingThoughtSignature:
		return model.ErrRequestRejected
	default:
		return model.ErrOutputWithheld
	}
}

// geminiTerminal returns the terminal a candidate finished with, when one
// delivered an answer. Intermediate stream chunks legitimately carry no
// finishReason — only the final chunk (and every non-stream response) does.
func geminiTerminal(out geminiResponse) (string, bool) {
	for _, cand := range out.Candidates {
		if cand.FinishReason == geminiStop || cand.FinishReason == geminiMaxTokens {
			return cand.FinishReason, true
		}
	}
	return "", false
}
