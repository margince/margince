// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// ReadableBasis is the basis a reader shows for a stored value. A row that
// holds the retired agreement, which rows written before the list became
// country-neutral may still hold, is shown as the consent it records. No writer
// produces that value and it is not a Basis. Every other value is shown as
// stored.
func ReadableBasis(stored string) string {
	if stored == "vn_subject_agreement" {
		return string(BasisConsent)
	}
	return stored
}
