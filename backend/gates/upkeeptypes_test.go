// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
)

// Each upkeep and maintenance subject is a record type some audit writer is
// called with. A stale or misspelt name keeps nothing out of the unplaced
// count, and lets the real subject back in.
func TestEveryUpkeepTypeIsAnAuditedSubject(t *testing.T) {
	t.Parallel()
	census := auditEntityCensusOf(t)
	if !census.seedInserts {
		t.Fatalf("%s no longer holds the INSERT into audit_log, so the census starts from nothing", auditSeedFunc)
	}
	written, _, _ := goAuditedRecordTypes(t, census)
	for _, subject := range append(auditentity.UpkeepTypes(), auditentity.MaintenanceTypes()...) {
		if !written[subject] {
			t.Errorf("no audit writer is called with the record type %q", subject)
		}
	}
}
