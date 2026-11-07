// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// SQL mirrors these codes because its audit trigger cannot call Go.
// TestFixedPrivacyReasonsMatchTheDatabase holds both directions of the mirror.
func fixedPrivacyReasons() []string {
	return []string{"retention", "test", "dsr_request", restrictionExpiredCause,
		untoldLeadReason, subjectRedactionReason, aLegalHoldEnded.collateralReason,
		theClockRanOut.collateralReason, aControllerDecided.collateralReason, aNoiseVerdictHid.collateralReason}
}
