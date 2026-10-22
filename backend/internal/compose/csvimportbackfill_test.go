// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/migrations"
)

// importSessionMigration is the custom migration whose backfill writes and
// matches the run causes below.
const importSessionMigration = "20261013120000"

// The sweep matches and the store writes the Go constants; the backfill spells
// the same sentences in SQL. A reworded constant would split the two silently.
func TestTheBackfillSpellsTheCausesTheCodeMatches(t *testing.T) {
	custom, err := migrations.Custom()
	if err != nil {
		t.Fatalf("loading custom migrations: %v", err)
	}
	var upSQL string
	for _, m := range custom.Migrations {
		if m.Version == importSessionMigration {
			upSQL = m.UpSQL
		}
	}
	if upSQL == "" {
		t.Fatalf("custom migration %s is missing", importSessionMigration)
	}
	for name, cause := range map[string]string{
		"CauseValidationStopped": migration.CauseValidationStopped,
		"CauseSuperseded":        migration.CauseSuperseded,
		"CauseStopped":           migration.CauseStopped,
		"CauseSourceTaken":       migration.CauseSourceTaken,
	} {
		if !strings.Contains(upSQL, "'"+cause+"'") {
			t.Errorf("the backfill does not spell migration.%s (%q); the run it writes or selects would drift from the code", name, cause)
		}
	}
}
