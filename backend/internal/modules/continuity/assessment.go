// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package continuity

// Verification separates a recorded drill outcome from proof that recovery targets were met.
type Verification struct {
	Code                 string
	RecoveryWithinTarget *bool
	DataLossWithinTarget *bool
}

// AssessDrill requires a passed receipt and both targets before declaring recovery verified.
func AssessDrill(drill *Drill) Verification {
	if drill == nil {
		return Verification{Code: "recovery_missing"}
	}
	out := Verification{Code: "recovery_unverified"}
	loss := drill.DataLossWindow() <= DataLossTarget
	out.DataLossWithinTarget = &loss
	if duration, known := drill.RecoveryWindow(); known {
		recovery := duration <= RecoveryTarget
		out.RecoveryWithinTarget = &recovery
		if drill.Outcome == OutcomePassed && recovery && loss {
			out.Code = "recovery_verified"
		}
	}
	return out
}
