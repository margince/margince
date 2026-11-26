// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestAnUntypedFileSaysHowItsTypeWasChosen(t *testing.T) {
	if object, how := ObjectOf(map[string]map[string]string{
		migration.ObjectContact: {"First Name": "first_name"}, migration.ObjectLead: {},
	}); object != migration.ObjectContact || how != crmcontracts.ImportSourceProfileObjectChosenByColumns {
		t.Errorf("own columns = %s by %s, want contact by columns", object, how)
	}
	if object, how := ObjectOf(map[string]map[string]string{
		migration.ObjectContact: {"Email": "email"}, migration.ObjectLead: {"Email": "email"},
	}); object != migration.ObjectLead || how != crmcontracts.ImportSourceProfileObjectChosenByNone {
		t.Errorf("shared columns = %s by %s, want lead by none", object, how)
	}
}
