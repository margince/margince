// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How the adapters build the port's two outcome sentinels,
// model.ErrOutputWithheld and model.ErrRequestRejected: a provider that
// declined to deliver the answer, or refused the request itself.

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// finishRefusal and finishContentFilter are terminals several wires spell the
// same way: Anthropic's stop_reason and OpenAI's content part type for a
// refusal, and OpenAI's and the broker wire's name for a filtered answer.
const (
	finishRefusal       = "refusal"
	finishContentFilter = "content_filter"
)

// withheldError is a withholding terminal that still NAMES itself, so the
// stored call says which filter fired and not merely that one did.
type withheldError struct {
	wire   string
	reason string
	// detail is text the provider chose, already passed through
	// safeProviderText where it came from a remote party.
	detail string
	// spent is the usage the withholding reply reported, which the provider
	// bills whether or not it delivered the answer.
	spent model.Response
}

func (e withheldError) Error() string {
	msg := "ai: " + e.wire + ": answer withheld: " + e.reason
	if e.detail != "" {
		msg += ": " + e.detail
	}
	return msg
}

// FinishReason satisfies the accessor finishReasonFor probes for.
func (e withheldError) FinishReason() string { return e.reason }

// Spent satisfies the accessor the ladder meters a withheld rung by.
func (e withheldError) Spent() model.Response { return e.spent }

func (e withheldError) Unwrap() error { return model.ErrOutputWithheld }

// withSpend attaches a withholding reply's usage to err when err is the
// adapter's own withheldError, and returns any other error unchanged.
func withSpend(err error, spent model.Response) error {
	var withheld withheldError
	if !errors.As(err, &withheld) {
		return err
	}
	withheld.spent = model.Response{
		InputTokens: spent.InputTokens, OutputTokens: spent.OutputTokens, CachedTokens: spent.CachedTokens,
		ReasoningTokens: spent.ReasoningTokens, CacheWriteTokens: spent.CacheWriteTokens,
	}
	return withheld
}

// truncatedError ends a stream the output ceiling cut off. A stream's port has
// no terminal to carry the truncation that Complete reports as a Response, so
// it arrives here instead, spelled as Complete spells it.
type truncatedError struct{ wire string }

func (e truncatedError) Error() string {
	return "ai: " + e.wire + ": the answer was cut off at the output ceiling"
}

// FinishReason satisfies the accessor finishReasonFor probes for.
func (truncatedError) FinishReason() string { return model.FinishReasonLength }

// rejectedRequest marks err as the vendor's own verdict that the request is
// malformed. Only a vendor error code can make that claim: a status cannot,
// because every vendor here also answers 400 for a context too long for one
// model, a parameter one model lacks, a bad key, an account or a region, and
// each of those is a reason to try the next rung.
func rejectedRequest(err error) error {
	return fmt.Errorf("%w: %w", model.ErrRequestRejected, err)
}

// openAIMalformedCodes are the OpenAI error codes that say the request is
// malformed as written, whichever model it reaches. Codes that depend on the
// model (context_length_exceeded, unsupported_parameter, unsupported_value)
// or on the account and key are left out on purpose.
var openAIMalformedCodes = map[string]bool{
	"invalid_json_schema":        true,
	"invalid_type":               true,
	"missing_required_parameter": true,
	"unknown_parameter":          true,
}

// openAIRejectsTheRequest reads an OpenAI-wire error code, which is a string on
// OpenAI and a number or absent on a broker; only a named malformed code counts.
func openAIRejectsTheRequest(code json.RawMessage) bool {
	var name string
	if json.Unmarshal(code, &name) != nil {
		return false
	}
	return openAIMalformedCodes[name]
}
