// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"fmt"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// ExtensionNamespaces turns a composed extension set into migration
// namespaces — one per unit that ships a migrations layer, each tracked in
// its own schema_migrations_ext_<name>.
//
// The bytes come from the unit's own embedded FS, so this works in the
// deployed image, where there is no extensions/ tree to read (the api image
// ships the binary alone).
//
// Sorted by unit name. No unit's schema may depend on another's — each owns
// only its ext_<name>_ tables — so the order is not a correctness
// requirement; it is that two runs of one composition must produce the same
// migration log, and the composed slice's order belongs to the generator.
//
// It lives here rather than in the migrate binary because two processes ask
// the same question of one set: the migrator, which applies these namespaces,
// and the serving process, which asks whether they were applied before it
// reports itself ready. A second derivation would be a second answer to
// "which namespaces does this composition have", and the two would disagree
// on exactly the unit somebody added and only wired into one of them.
func ExtensionNamespaces(exts []extension.Extension) ([]Namespace, error) {
	ordered := make([]extension.Extension, len(exts))
	copy(ordered, exts)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })

	namespaces := make([]Namespace, 0, len(ordered))
	for _, e := range ordered {
		if e.Migrations == nil {
			continue // a unit that owns no tables, which is the common case
		}
		// NamespaceFor, not a local derivation: the tracking table, the
		// ext_<name>_ table prefix and the ext_<name> role are ONE namespace,
		// and a second spelling is how they start disagreeing. It validates
		// the unit name too, so a name that could not be a SQL identifier is
		// refused before any DDL runs.
		namespace, err := NamespaceFor(string(e.Name))
		if err != nil {
			return nil, fmt.Errorf("pgmigrate: extension %q: %w", e.Name, err)
		}
		loaded, err := Load(e.Migrations, extension.MigrationsDir)
		if err != nil {
			return nil, fmt.Errorf("pgmigrate: extension %q: %w", e.Name, err)
		}
		if len(loaded) == 0 {
			return nil, fmt.Errorf("pgmigrate: extension %q embeds %s/ but it holds no NNNN_name.up.sql/.down.sql pair — a declared-but-empty layer reads as a schema that applied, so leave Migrations nil for a unit that owns no tables", e.Name, extension.MigrationsDir)
		}
		namespaces = append(namespaces, Namespace{Name: namespace, Migrations: loaded})
	}
	return namespaces, nil
}
