// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Wired at link time, so every binary that reads a signal words it alike. An
// unwired reader would serve the stored text.
func init() { signals.WordSummariesWith(signalSummaryWords{}) }

// signalSummaryWords words a summary again from the parts its producer stored,
// through the same function that producer wrote it with.
type signalSummaryWords struct{}

func (signalSummaryWords) ReaderLanguage(ctx context.Context, tx pgx.Tx) textlang.Lang {
	return identity.ReaderLanguageForRecord(ctx, tx)
}

func (signalSummaryWords) Word(lang textlang.Lang, kind string, params json.RawMessage) (string, bool) {
	switch kind {
	case kindTechnicalChange:
		parts, ok := decodeSummaryParts[technicalChangeParts](params)
		if !ok || !parts.complete() {
			return "", false
		}
		return parts.worded(lang)
	case kindGhostedThread:
		parts, ok := decodeSummaryParts[ghostedThreadParts](params)
		if !ok || !parts.complete() {
			return "", false
		}
		return parts.worded(lang), true
	case kindProjectGoneQuiet:
		parts, ok := decodeSummaryParts[projectQuietParts](params)
		if !ok || !parts.complete() {
			return "", false
		}
		return parts.worded(lang), true
	default:
		return "", false
	}
}

// decodeSummaryParts refuses a field the parts do not declare, so parts of
// another shape fall back to the stored summary instead of a half-worded one.
func decodeSummaryParts[P technicalChangeParts | ghostedThreadParts | projectQuietParts](params json.RawMessage) (P, bool) {
	var parts P
	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parts); err != nil {
		return parts, false
	}
	return parts, true
}
