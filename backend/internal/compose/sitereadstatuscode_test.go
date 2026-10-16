// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// A failed read carries one of the closed failure codes. Both read shapes must
// name every one, or a validator rejects the answer that says why a read failed.
func TestEverySiteReadFailureCodeIsDeclaredOnBothReadShapes(t *testing.T) {
	t.Parallel()
	for code := range contacts.SiteReadFailureCodes {
		if !crmcontracts.SiteReadReportStatusCode(code).Valid() {
			t.Errorf("SiteReadReport.status_code does not declare %q", code)
		}
		if !crmcontracts.CompanySiteReadStatusCode(code).Valid() {
			t.Errorf("CompanySiteRead.status_code does not declare %q", code)
		}
	}
	if !crmcontracts.SiteReadReportStatusCode(string(crmcontracts.SiteReadReportStatusCodeBudgetDeferred)).Valid() {
		t.Error("budget_deferred fell out of SiteReadReport.status_code")
	}
}
