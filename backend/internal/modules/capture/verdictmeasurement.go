// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// VerdictMeasurement is how a model-made answer was reached: how sure it was,
// and which model served it.
//
// Neither survived the decision before this. The engine compared the confidence
// against a floor and dropped it, so afterwards a 0.71 answer and a 0.99 one
// were indistinguishable — an operator asking why a department was filed as a
// person could see the answer and never how close it came to being refused. The
// served model matters for the same reason: this lane runs on whatever local
// model a deployment bound, and a wrong answer is evidence about that model only
// if the model is named.
//
// The zero value means no model was asked, which is a real and common case: an
// owner's own decision and a role mailbox are answered from the address and the
// ledger. It stores NULL rather than zero, because a stored 0.0 would read as a
// model that was certain of nothing rather than as no model at all.
type VerdictMeasurement struct {
	Confidence float64
	Model      string
	// Asked distinguishes "a model answered with confidence 0" from "no model
	// was asked". Without it the zero Confidence is ambiguous, and the ambiguity
	// lands in a column an operator reads.
	Asked bool
}

// MeasuredVerdict records what a model answered.
func MeasuredVerdict(confidence float64, model string) VerdictMeasurement {
	return VerdictMeasurement{Confidence: confidence, Model: model, Asked: true}
}

// confidence is the value the column takes: the measurement, or NULL when no
// model was asked.
func (m VerdictMeasurement) confidence() *float64 {
	if !m.Asked {
		return nil
	}
	return &m.Confidence
}
