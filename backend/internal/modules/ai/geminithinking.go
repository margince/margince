// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How much a Gemini request asks the model to think: the level a structured
// request defaults to, and which models may be named a level at all.

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// geminiStructuredThinkingLevel is the thinking level a schema-constrained
// request gets when the caller named none.
//
// Gemini counts thinking against maxOutputTokens, and each model's own default
// is deep — gemini-3.1-pro-preview thinks at high, gemini-3.5-flash at medium —
// so a structured lane with a modest cap could spend all of it thinking and
// return no answer at all. Measured on a three-field email classification under
// a 400-token cap, two runs per model and level: at each model's default level
// all four runs stopped at MAX_TOKENS having spent 381-384 tokens thinking and
// written 0-5 of the answer; at low, three of four finished, thinking 175-310.
// A schema already says what the answer is shaped like, which is most of what
// the thinking was for. The run that still ran out — pro, 359 tokens thinking —
// is what the structured retry's room-to-answer lever is for (structured.go).
//
// "low" rather than the shallower "minimal" because it is the floor every
// Gemini model this tree binds accepts: gemini-3.1-pro-preview answers minimal
// with a 400.
//
// It only ever LOWERS a model's thinking, so a model whose own default is
// already at or below it is sent no level (geminiThinksShallowByDefault).
const geminiStructuredThinkingLevel = "low"

// geminiThinksShallowByDefault reports whether a model's own thinking default
// is already no deeper than geminiStructuredThinkingLevel, so naming that level
// would raise it. Google's thinking table puts every Flash-Lite there:
// gemini-3.1-flash-lite and gemini-3.5-flash-lite default to minimal,
// gemini-2.5-flash-lite does not think at all unless asked, and
// gemini-3.1-flash-lite-image accepts only minimal and high, so low is a 400
// there besides.
//
// Keyed on the family name rather than a list of ids because the table is per
// family: a new Flash-Lite ships at the cheap end of the scale by design, and
// an unlisted id falling through to "low" would re-open the raise this exists
// to prevent.
func geminiThinksShallowByDefault(model string) bool {
	return strings.Contains(model, "flash-lite")
}

// geminiRaisedToFloor is the level a request that names none of its own is
// sent: level (the adapter's default, empty for the model's own) raised to
// floor where that default is shallower. Flash-Lite thinks at minimal by
// default and every other Gemini 3 at medium or deeper, so medium is assumed:
// naming high to a model already there is harmless. Pre-3 is sent nothing.
func geminiRaisedToFloor(modelID, level, floor string) string {
	if floor == "" || !geminiTakesThinkingLevel(modelID) {
		return level
	}
	effective := level
	if effective == "" {
		effective = effortMedium
		if geminiThinksShallowByDefault(modelID) {
			effective = effortMinimal
		}
	}
	if effortAtLeast(effective, floor) {
		return level
	}
	return floor
}

// geminiTakesThinkingLevel reports whether a model accepts
// thinkingConfig.thinkingLevel on generateContent. Google's reference for the
// field: "Recommended for Gemini 3 or later models. Use with earlier models
// results in an error." — a Gemini 2.5 takes only thinkingBudget there, so the
// structured default is not named to one. (The Interactions API's thinking
// guide lists levels for 2.5; that is a different surface from this one.)
//
// Keyed on the generation number, and an id that names none — an alias such as
// gemini-flash-latest — counts as current, because aliases track the newest
// generation and the error is confined to generations that predate the field.
// A level the CALLER chose is sent as asked: the vendor's refusal of it is the
// honest answer, and dropping it here would hide that it was never applied.
func geminiTakesThinkingLevel(model string) bool {
	id := strings.TrimPrefix(strings.TrimPrefix(model, "models/"), "gemini-")
	digits := len(id) - len(strings.TrimLeft(id, "0123456789"))
	generation, err := strconv.Atoi(id[:digits])
	if err != nil {
		return true
	}
	return generation >= 3
}

type geminiThinking struct {
	ThinkingLevel string `json:"thinkingLevel"` //nolint:tagliatelle // Google's wire format (camelCase)
}

// geminiThinkingLevels is the thinkingLevel vocabulary generateContent takes,
// shallowest first. Which of them one model accepts is the vendor's to say —
// gemini-3.1-pro-preview refuses minimal — so the parser checks the word and
// leaves the pairing to the vendor's 400.
var geminiThinkingLevels = []string{"minimal", "low", "medium", "high"} //nolint:goconst // Google's vocabulary; the same words in the broker's and Ollama's lists belong to other vendors and must not move with it

// thinkingLevelDefault is the `thinking_level` a routing save sends to clear a
// stored level; the store turns it into no level before anything validates it.
const thinkingLevelDefault = "default"

// validateThinkingLevel refuses a binding's `thinking_level` that no request
// could carry: on a provider other than gemini, outside the vocabulary, or on a
// model that predates the field. Refused at load rather than sent, because the
// last two fail every call and the first would be ignored in silence.
func validateThinkingLevel(lane string, binding ProviderConfig) error {
	level := binding.ThinkingLevel
	switch {
	case level == "":
		return nil
	case binding.Provider != providerGemini:
		return fmt.Errorf("ai: routing config: %s: `thinking_level` is Gemini's thinkingConfig and provider %s has no such field; remove it",
			lane, binding.Provider)
	case !slices.Contains(geminiThinkingLevels, level):
		return fmt.Errorf("ai: routing config: %s: thinking_level %q is not one of %s",
			lane, level, strings.Join(geminiThinkingLevels, " | "))
	case !geminiTakesThinkingLevel(binding.Model):
		return fmt.Errorf("ai: routing config: %s: model %s predates thinkingLevel and answers it with a 400; remove thinking_level or bind a Gemini 3 model",
			lane, binding.Model)
	}
	return nil
}
