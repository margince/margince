// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How a Gemini reply ends: the finishReason every candidate carries, read the
// same way by Complete and by the stream.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// geminiResponseError maps an in-body error object or a finishReason that
// delivers no answer to an error. Gemini delivers SAFETY, RECITATION, … inside
// a 200 body, so unchecked they would pass for a complete answer. STOP and
// MAX_TOKENS are not errors: both carry the text generated, and absent is a
// non-final stream chunk.
func geminiResponseError(ctx context.Context, out geminiResponse) error {
	if out.Error.Message != "" || out.Error.Status != "" {
		return fmt.Errorf("ai: gemini: %s: %s", safeProviderText(ctx, out.Error.Status), safeProviderText(ctx, out.Error.Message))
	}
	// A blocked PROMPT has no candidate to carry a finishReason at all: the
	// reason is under promptFeedback, and without reading it the reply looks
	// like a body cut short in transit.
	if len(out.Candidates) == 0 && out.PromptFeedback.BlockReason != "" {
		reason := out.PromptFeedback.BlockReason
		if !terminalCode.MatchString(reason) {
			reason = "OTHER"
		}
		return withheldError{wire: providerGemini, reason: geminiPromptBlocked + reason, detail: "the prompt was blocked"}
	}
	for _, cand := range out.Candidates {
		switch reason := cand.FinishReason; {
		case reason == "", reason == geminiStop, reason == geminiMaxTokens:
		case geminiWithholds(reason):
			return withheldError{wire: providerGemini, reason: reason}
		default:
			// OTHER, MALFORMED_FUNCTION_CALL, MISSING_THOUGHT_SIGNATURE and the
			// rest are this reply going wrong, not a decision about the
			// content, so they fail the call and the ladder walks on.
			return fmt.Errorf("ai: gemini: generation stopped: %s", safeProviderText(ctx, reason))
		}
	}
	return nil
}

// geminiStop and geminiMaxTokens are the two terminals that deliver an answer:
// whole, or cut off at maxOutputTokens.
const (
	geminiStop      = "STOP"
	geminiMaxTokens = "MAX_TOKENS"
	// geminiPromptBlocked prefixes a promptFeedback blockReason, so a blocked
	// prompt and a blocked answer are separate terminals in a stored call.
	geminiPromptBlocked = "PROMPT_BLOCKED_"
)

// geminiWithheldReasons are the finishReasons that are a filter's decision
// about the content. Positive on purpose: a terminal Google adds later is a
// failed call that walks, not an outcome that ends one.
var geminiWithheldReasons = map[string]bool{
	"SAFETY": true, "RECITATION": true, "BLOCKLIST": true, "PROHIBITED_CONTENT": true, "SPII": true,
	"IMAGE_SAFETY": true, "IMAGE_PROHIBITED_CONTENT": true, "IMAGE_RECITATION": true,
}

// geminiWithholds reports whether a terminal withheld the answer.
func geminiWithholds(reason string) bool {
	return geminiWithheldReasons[reason] || strings.HasPrefix(reason, geminiPromptBlocked)
}

// geminiFinishReason is Gemini's terminal in the port's vocabulary: a cut-off
// reply is model.FinishReasonLength on every wire, because the structured
// retry and the cert lane's ungraded run read that value and no other.
func geminiFinishReason(reason string) string {
	if reason == geminiMaxTokens {
		return model.FinishReasonLength
	}
	return reason
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
