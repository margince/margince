// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "testing"

func TestAStoredRetiredAgreementReadsAsConsent(t *testing.T) {
	t.Parallel()
	if got := ReadableBasis("vn_subject_agreement"); got != string(BasisConsent) {
		t.Errorf("ReadableBasis(retired agreement) = %q, want consent", got)
	}
	for _, basis := range []Basis{BasisContract, BasisConsent, BasisExistingCustomerException} {
		if got := ReadableBasis(string(basis)); got != string(basis) {
			t.Errorf("ReadableBasis(%q) = %q, want it unchanged", basis, got)
		}
	}
}
