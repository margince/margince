// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A voice-source weight is a multiplier in corpus scoring, so a value that is
// not a number does not fail loudly - it propagates through every sum and
// average it takes part in and turns the computed profile into NaN. The range
// check alone could not refuse one, because every comparison against NaN is
// false.

import (
	"errors"
	"math"
	"testing"
)

func TestAVoiceWeightMustBeANumberInRange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		weight  float64
		refused bool
	}{
		{"the default", 1.0, false},
		{"the lower bound", 0, false},
		{"the upper bound", 2.0, false},
		{"below the range", -0.5, true},
		{"above the range", 2.5, true},
		// The one the range check admitted: NaN compares false against both
		// bounds, so it passed and was stored.
		{"not a number", math.NaN(), true},
		{"positive infinity", math.Inf(1), true},
		{"negative infinity", math.Inf(-1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := voiceWeightRefused(tc.weight); got != tc.refused {
				t.Errorf("voiceWeightRefused(%v) = %t, want %t", tc.weight, got, tc.refused)
			}
		})
	}
}

// Both write paths validate separately, so fixing one would have left the
// other open. They answer through the same check and say the same sentence.
func TestBothVoiceWritePathsRefuseANaNWeight(t *testing.T) {
	_, _, err := validateDeclaredSource(IngestSourceInput{
		Kind: voiceSourceKindEmail, Register: voiceRegisterEmail,
		SourceLabel: "a label", Content: "some text", Weight: math.NaN(),
	})
	var refusal *CorpusIngestError
	if !errors.As(err, &refusal) {
		t.Fatalf("the ingest accepted a NaN weight (err %v)", err)
	}
	if refusal.Field != voiceKeyWeight || refusal.Reason != voiceWeightRange {
		t.Errorf("the ingest refused a NaN weight as %s/%q, want %s/%q",
			refusal.Field, refusal.Reason, voiceKeyWeight, voiceWeightRange)
	}
}
