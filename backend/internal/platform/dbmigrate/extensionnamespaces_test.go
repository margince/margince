// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// Which namespaces a composition HAS — asked of the mapping rather than of a
// database, because two processes read this answer (the migrator applies the
// namespaces, the serving process checks them) and a unit that mapped
// differently for one of them would be invisible to the other.

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/margince/margince/backend/pkg/extension"
)

// unitFS builds the filesystem shape a unit's `//go:embed migrations`
// produces: the layer directory sitting at the root of the FS.
func unitFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[extension.MigrationsDir+"/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestExtensionNamespacesSkipsAUnitThatOwnsNoTables(t *testing.T) {
	got, err := ExtensionNamespaces([]extension.Extension{{Name: "yogi", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("extensionNamespaces: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a unit with no Migrations produced %d namespace(s), want none — declaring no schema is the common case, not an error", len(got))
	}
}

func TestExtensionNamespacesMapsAUnitOntoItsExtNamespace(t *testing.T) {
	got, err := ExtensionNamespaces([]extension.Extension{{
		Name:    "foo-1",
		Version: "1.0.0",
		Migrations: unitFS(map[string]string{
			"0001_note.up.sql":   "CREATE TABLE ext.ext_foo_1_note (id int)",
			"0001_note.down.sql": "DROP TABLE ext.ext_foo_1_note",
		}),
	}})
	if err != nil {
		t.Fatalf("extensionNamespaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d namespaces, want 1", len(got))
	}
	// The hyphen→underscore mapping is NamespaceFor's, and this
	// pins that migrate goes through it rather than deriving its own: the
	// tracking table and the ext_<name> role must name one namespace.
	if got[0].Name != "ext_foo_1" {
		t.Errorf("namespace = %q, want %q", got[0].Name, "ext_foo_1")
	}
	if len(got[0].Migrations) != 1 || got[0].Migrations[0].Version != "0001" {
		t.Errorf("migrations = %+v, want the single 0001 pair", got[0].Migrations)
	}
}

func TestExtensionNamespacesOrdersByUnitNameNotCompositionOrder(t *testing.T) {
	layer := unitFS(map[string]string{
		"0001_t.up.sql":   "SELECT 1",
		"0001_t.down.sql": "SELECT 1",
	})
	got, err := ExtensionNamespaces([]extension.Extension{
		{Name: "zulu", Version: "1.0.0", Migrations: layer},
		{Name: "alpha", Version: "1.0.0", Migrations: layer},
	})
	if err != nil {
		t.Fatalf("extensionNamespaces: %v", err)
	}
	var names []string
	for _, ns := range got {
		names = append(names, ns.Name)
	}
	want := []string{"ext_alpha", "ext_zulu"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v — two runs of one composition must produce the same migration log", names, want)
	}
}

// TestExtensionNamespacesRefusesADeclaredButEmptyLayer covers the guard in
// extensionNamespaces, which needs a layer that EXISTS and holds no pair —
// a README-only directory is the real shape of that. An FS with no
// migrations/ directory at all does not reach the guard: Load
// fails first on fs.ErrNotExist, which is the case below this one.
func TestExtensionNamespacesRefusesADeclaredButEmptyLayer(t *testing.T) {
	_, err := ExtensionNamespaces([]extension.Extension{{
		Name: "hollow", Version: "1.0.0",
		Migrations: unitFS(map[string]string{"README.md": "how this unit's schema works"}),
	}})
	if err == nil {
		t.Fatal("an embedded migrations layer holding no pair was accepted — it reads as a schema that applied")
	}
	if !strings.Contains(err.Error(), "hollow") {
		t.Errorf("error %q does not name the offending unit", err)
	}
	// Pinned on the guard's own words, so a future refactor that lets
	// Load answer this case instead cannot pass silently: the two
	// messages tell an author different things to do.
	if !strings.Contains(err.Error(), "leave Migrations nil") {
		t.Errorf("error %q is not the declared-but-empty guard — it must say what to do instead", err)
	}
}

// TestExtensionNamespacesRefusesAMissingLayer is the botched-embed case: a
// unit sets Migrations to an FS that carries no migrations/ directory. It
// must fail loudly rather than read as a unit that owns no tables — leaving
// the field nil is how a unit says that.
func TestExtensionNamespacesRefusesAMissingLayer(t *testing.T) {
	_, err := ExtensionNamespaces([]extension.Extension{{
		Name: "misembedded", Version: "1.0.0", Migrations: fstest.MapFS{},
	}})
	if err == nil {
		t.Fatal("a Migrations FS with no migrations/ directory was accepted")
	}
	if !strings.Contains(err.Error(), "misembedded") {
		t.Errorf("error %q does not name the offending unit", err)
	}
}

func TestExtensionNamespacesRefusesAnUnmappableUnitName(t *testing.T) {
	_, err := ExtensionNamespaces([]extension.Extension{{
		Name: "Bad Name", Version: "1.0.0", Migrations: unitFS(map[string]string{
			"0001_t.up.sql":   "SELECT 1",
			"0001_t.down.sql": "SELECT 1",
		}),
	}})
	if err == nil {
		t.Fatal("a unit name that cannot be a SQL identifier was accepted — the namespace is interpolated into DDL")
	}
	if !strings.Contains(err.Error(), "Bad Name") {
		t.Errorf("error %q does not name the offending unit", err)
	}
}

func TestExtensionNamespacesRefusesAMigrationThatCannotBeReverted(t *testing.T) {
	_, err := ExtensionNamespaces([]extension.Extension{{
		Name: "oneway", Version: "1.0.0", Migrations: unitFS(map[string]string{
			"0001_t.up.sql": "SELECT 1",
		}),
	}})
	if err == nil {
		t.Fatal("a migration with no .down.sql was accepted")
	}
	if !strings.Contains(err.Error(), "oneway") {
		t.Errorf("error %q does not name the offending unit", err)
	}
}
