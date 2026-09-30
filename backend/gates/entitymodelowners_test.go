// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package gates

// The ownership map and the schema name the same tables.
//
// tableOwners is the hand-maintained half of table ownership, and it is read by
// six gates plus the entity model. Both directions of the difference are real
// defects, and they fail differently:
//
//   - A table in the schema with no owner is a table whose writes nothing
//     places. The entity model files it under the platform, which is right for
//     the write backbone's own tables and wrong for a module's — and wrong
//     SILENTLY, since the page renders either way.
//   - An owner naming a table the schema does not have is a claim about
//     something gone. `deal_room_release` sat here for months after the
//     migration that dropped it, and every reader of the map was told the
//     dealrooms module owned a table no database has.
//
// The second direction is the one nothing else could catch. tableownership's
// walk starts from writes it FINDS in the tree, so a table nobody writes any
// more is a table it never asks about.

import (
	"sort"
	"testing"
)

// platformOwned are the tables the write backbone and the runtime keep for
// themselves rather than any module owning: the audit and outbox rows every
// write commits, the logs, and the two reference tables the platform reads.
//
// Named here rather than added to tableOwners because that map answers "which
// module's store may write this", and the honest answer for these is none — the
// row is written by storekit inside somebody else's transaction, or by the
// runtime. Putting a module in the map would license a write nobody wants.
var platformOwned = map[string]bool{
	"audit_log":             true,
	"currency_minor_digits": true,
	"event_outbox":          true,
	"field_provenance":      true,
	"system_log":            true,
	"vault_secret":          true,
}

// notMigrationBuilt are tables a module owns that our migrations do not create,
// so the committed catalog cannot carry them. Today that is River's own job
// table: the queue library runs its own migrator, and a core migration that
// touches the table guards on its existence for exactly this reason.
//
// The exemption is by TABLE and carries its reason, rather than being a rule
// about a prefix. A prefix would quietly excuse the next table somebody named
// river_something and forgot to migrate.
// gatekit:fixture the tables our migrations do not create, each with why
var notMigrationBuilt = map[string]string{
	"river_job": "created by River's own migrator, not by migrations/core",
}

func TestEveryTableTheSchemaHasIsPlacedAndEveryPlacedTableExists(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)

	var unplaced []string
	for _, name := range schema.tableNames() {
		if _, owned := tableOwners[name]; owned || platformOwned[name] {
			continue
		}
		unplaced = append(unplaced, name)
	}
	if len(unplaced) > 0 {
		t.Errorf("%d table(s) in %s have no owner in tableOwners and are not declared platform-owned, "+
			"so the entity model files them under the platform without anybody having said so: %v\n"+
			"Add the owning module to tableOwners, or add the table to platformOwned with the "+
			"reason it belongs to no module.", len(unplaced), headCatalogPath, unplaced)
	}

	var vanished []string
	for table := range tableOwners {
		if _, exists := schema.tables[table]; exists || notMigrationBuilt[table] != "" {
			continue
		}
		vanished = append(vanished, table)
	}
	sort.Strings(vanished)
	if len(vanished) > 0 {
		t.Errorf("tableOwners claims %d table(s) the schema does not have, so every reader of the "+
			"map is told a module owns something no database holds: %v\n"+
			"A migration dropped them; drop their entries too.", len(vanished), vanished)
	}

	if len(schema.tables) < catalogTableFloor {
		t.Fatalf("this census judged %d table(s), below the floor of %d — it has stopped reading "+
			"the schema and would certify an empty one", len(schema.tables), catalogTableFloor)
	}
}
