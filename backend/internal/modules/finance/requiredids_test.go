// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
)

// A link body that omits company_id is refused as a missing argument, not
// looked up as a company that does not exist.
func TestALinkBodyWithoutACompanyIsNamedWhenAbsent(t *testing.T) {
	var req crmcontracts.LinkFinanceSourceCustomerRequest
	if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
		t.Fatal(err)
	}
	_, err := linkCompany(req)
	faulttest.AssertNamesOmittedID(t, err, "company_id")
}
