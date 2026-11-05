// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Voice reason codes keep client explanations independent of stored prose.
const (
	VoiceReasonUncomparable   = "uncomparable"
	VoiceReasonIdentityShift  = "identity_shift"
	VoiceReasonSignatureShift = "signature_shift"
	VoiceReasonRulesRemoved   = "rules_removed"
	VoiceReasonHardFailures   = "hard_failures"
	VoiceReasonMalformed      = "malformed"
	VoiceReasonLowScore       = "low_score"
	VoiceReasonRegression     = "regression"
	VoiceReasonTooSmall       = "too_small"
	VoiceReasonCorpusChanged  = "corpus_changed"
	VoiceReasonActiveChanged  = "active_changed"
)
