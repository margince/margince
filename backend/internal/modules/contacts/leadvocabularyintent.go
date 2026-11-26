// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/shared/kernel/values"

// leadVocabularyObject is the RBAC object the lead lists are gated by: the
// custom-field catalog's posture, where everyone reads and admin or ops write.
const leadVocabularyObject = "custom_field"

// SourceIntent is what the scorer makes of a lead's origin. It keeps the
// spelling of the contract's LeadSourceIntent, which lead origins use.
type SourceIntent string

// The three intents a lead origin carries.
const (
	SourceIntentHigh    SourceIntent = "high"
	SourceIntentNeutral SourceIntent = "neutral"
	SourceIntentLow     SourceIntent = "low"
)

// ParseSourceIntent is the seam guard for the three-value intent.
func ParseSourceIntent(raw string) (SourceIntent, error) {
	switch s := SourceIntent(raw); s {
	case SourceIntentHigh, SourceIntentNeutral, SourceIntentLow:
		return s, nil
	}
	return "", &values.ParseError{Field: "intent", Code: "invalid_intent", Message: "intent is one of high, neutral, low"}
}
