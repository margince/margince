// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// openaiReplyText walks output[], never output[0].content[0]: a reasoning
// item can precede the message, and a refusal part is an outcome of its own.
// A schema-bound reply is the first message with text, since a model can add
// a second one and the two joined are not one document. Once a final_answer
// exists, commentary messages are working notes and are skipped.
func openaiReplyText(ctx context.Context, out openaiResponse, schemaBound bool) (string, error) {
	skipCommentary := slices.ContainsFunc(out.Output, func(item openaiOutputItem) bool { return item.Phase == "final_answer" })
	var text strings.Builder
	for _, item := range out.Output {
		if item.Type != "message" || (skipCommentary && item.Phase == "commentary") {
			continue
		}
		if schemaBound && text.Len() > 0 {
			break
		}
		for _, part := range item.Content {
			switch part.Type {
			case "output_text":
				text.WriteString(part.Text)
			case finishRefusal:
				return "", withheldError{wire: providerOpenAI, reason: finishRefusal, detail: safeProviderText(ctx, part.Refusal)}
			}
		}
	}
	return text.String(), nil
}

// openaiCutOff reports a response the output ceiling stopped: an answer,
// truncated, rather than a failed call. Only Complete reads it; a stream ends
// on truncatedError, the port's model.ErrOutputTruncated.
func openaiCutOff(out openaiResponse) bool {
	return out.Status == "incomplete" && out.IncompleteDetails.Reason == openaiMaxOutputTokens
}

// openaiTerminalStatus maps a non-completed Responses object to an error: a
// failed call carries the API's error, an incomplete one names why generation
// stopped (max_output_tokens, content_filter), and a missing status means the
// body was not a terminal Responses object at all. Any of them read as a clean
// answer would hand the caller a truncated or filtered result, so
// "completed" is the only success.
func openaiTerminalStatus(ctx context.Context, out openaiResponse) error {
	switch out.Status {
	case "completed":
		return nil
	case "failed":
		return openaiFailure(ctx, out.Error.Code, out.Error.Message)
	case "incomplete":
		switch out.IncompleteDetails.Reason {
		case finishContentFilter:
			return withheldError{wire: providerOpenAI, reason: finishContentFilter}
		case openaiMaxOutputTokens:
			return truncatedError{wire: providerOpenAI}
		}
		return fmt.Errorf("ai: openai: response incomplete: %s", safeProviderText(ctx, out.IncompleteDetails.Reason))
	case "":
		return fmt.Errorf("ai: openai: response carries no terminal status")
	default:
		return fmt.Errorf("ai: openai: response ended with status %q", safeProviderText(ctx, out.Status))
	}
}

// openaiFailure is the error for a failed response's code, on the body or as a
// stream's error event: a rate limit is the throttle a 429 is.
func openaiFailure(ctx context.Context, code, message string) error {
	if openaiPolicyCodes[code] {
		return withheldError{wire: providerOpenAI, reason: code, detail: safeProviderText(ctx, message)}
	}
	err := fmt.Errorf("ai: openai: response failed: %s: %s", safeProviderText(ctx, code), safeProviderText(ctx, message))
	if code == "rate_limit_exceeded" {
		return refusedAs(refusalThrottle, err)
	}
	return err
}

// openaiPolicyCodes are the codes, on a failed response or a 400, that are a
// policy decision about the content, so the answer is withheld rather than the
// call failed.
var openaiPolicyCodes = map[string]bool{"invalid_prompt": true, "bio_policy": true, "misalignment_policy_violation": true}

// openaiMaxOutputTokens is the incomplete reason for a reply the output
// ceiling cut off.
const openaiMaxOutputTokens = "max_output_tokens"
