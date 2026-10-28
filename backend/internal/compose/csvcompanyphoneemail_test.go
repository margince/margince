// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

// A company import maps the main number and the general address, and an
// unchanged file compares equal to what the store holds.
func TestACompanyImportMapsItsPhoneAndEmail(t *testing.T) {
	t.Parallel()
	for _, target := range []string{fieldPhone, fieldEmail} {
		if !slices.Contains(csvTargets[migration.ObjectCompany], target) {
			t.Errorf("a company import offers no %s column", target)
		}
	}
	if got := canonicalFor(fieldPhone, " +49 30 1234567 "); got != "+49301234567" {
		t.Errorf("canonical phone = %q, want the stored E.164 spelling", got)
	}
	if got := canonicalFor(fieldPhone, "call the office"); got != "call the office" {
		t.Errorf("an unparseable phone compares as %q, want it as written", got)
	}
	in := companyCreateFrom(map[string]string{fieldDisplayName: "Acme", fieldPhone: "+49 30 1", fieldEmail: "info@acme.example"}, nil, "csv", "import")
	if in.Phone == nil || in.Email == nil {
		t.Errorf("the create input carries phone %v and email %v, want both", in.Phone, in.Email)
	}
	patch := companyUpdateFrom(map[string]string{fieldPhone: "+49 30 2"}, nil)
	if patch.Phone == nil || patch.Email != nil {
		t.Errorf("the patch carries phone %v and email %v, want only the changed phone", patch.Phone, patch.Email)
	}
}
