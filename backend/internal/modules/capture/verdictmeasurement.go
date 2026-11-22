// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// VerdictMeasurement is how sure a model-made answer was. The ledger keeps it
// so a 0.71 answer and a 0.99 one stay apart after the floor judged them.
//
// The zero value means no model was asked, which is a real and common case: an
// owner's own decision and a role mailbox are answered from the address and the
// ledger. It stores NULL rather than zero, because a stored 0.0 would read as a
// model that was certain of nothing rather than as no model at all.
type VerdictMeasurement struct {
	Confidence float64
	// Asked distinguishes "a model answered with confidence 0" from "no model
	// was asked". Without it the zero Confidence is ambiguous, and the ambiguity
	// lands in a column an operator reads.
	Asked bool
}

// MeasuredVerdict records what a model answered.
func MeasuredVerdict(confidence float64) VerdictMeasurement {
	return VerdictMeasurement{Confidence: confidence, Asked: true}
}

// confidence is the value the column takes: the measurement, or NULL when no
// model was asked.
func (m VerdictMeasurement) confidence() *float64 {
	if !m.Asked {
		return nil
	}
	return &m.Confidence
}
