// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoad_pairsAndOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"core/0002_second.up.sql":   {Data: []byte("CREATE TABLE b ();")},
		"core/0002_second.down.sql": {Data: []byte("DROP TABLE b;")},
		"core/0001_first.up.sql":    {Data: []byte("CREATE TABLE a ();")},
		"core/0001_first.down.sql":  {Data: []byte("DROP TABLE a;")},
		"core/README.md":            {Data: []byte("ignored")},
	}

	ms, err := Load(fsys, "core")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("got %d migrations, want 2", len(ms))
	}
	if ms[0].Version != "0001" || ms[1].Version != "0002" {
		t.Errorf("order = %s, %s; want 0001, 0002", ms[0].Version, ms[1].Version)
	}
	if ms[0].Name != "first" || ms[0].UpSQL == "" || ms[0].DownSQL == "" {
		t.Errorf("0001 loaded incompletely: %+v", ms[0])
	}
}

func TestLoad_rejectsIrreversibleMigration(t *testing.T) {
	fsys := fstest.MapFS{
		"core/0001_first.up.sql": {Data: []byte("CREATE TABLE a ();")},
	}
	_, err := Load(fsys, "core")
	if err == nil || !strings.Contains(err.Error(), "both .up.sql and .down.sql") {
		t.Fatalf("err = %v, want missing-down error", err)
	}
}

func TestLoad_rejectsUnversionedName(t *testing.T) {
	fsys := fstest.MapFS{
		"core/nodash.up.sql":   {Data: []byte("SELECT 1;")},
		"core/nodash.down.sql": {Data: []byte("SELECT 1;")},
	}
	_, err := Load(fsys, "core")
	if err == nil {
		t.Fatal("Load accepted a migration without <version>_<name>")
	}
}

func TestNamespaceFor(t *testing.T) {
	cases := []struct {
		unit    string
		want    string
		wantErr string
	}{
		{unit: "foo-1", want: "ext_foo_1"},
		{unit: "notes", want: "ext_notes"},
		{unit: "yogi", want: "ext_yogi"},
		// The refusals all come from the ONE published name rule; this
		// function adds none of its own, so these pin that it validates
		// rather than that it re-implements.
		{unit: "Bad-Name", wantErr: "not a valid unit name"},
		{unit: "trailing-", wantErr: "not a valid unit name"},
		{unit: "", wantErr: "not a valid unit name"},
		{unit: strings.Repeat("a", 33), wantErr: "capped at 32"},
	}
	for _, tc := range cases {
		got, err := NamespaceFor(tc.unit)
		switch {
		case tc.wantErr != "":
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("NamespaceFor(%q) err = %v, want %q", tc.unit, err, tc.wantErr)
			}
		case err != nil:
			t.Errorf("NamespaceFor(%q) = %v", tc.unit, err)
		case got != tc.want:
			t.Errorf("NamespaceFor(%q) = %q, want %q", tc.unit, got, tc.want)
		}
	}
}

// TestNamespaceForFitsTheTrackingTableGrammar closes the loop that made this
// mapping worth writing here rather than at the call site: a derived
// namespace must be spellable as schema_migrations_<ns>, digits and all.
func TestNamespaceForFitsTheTrackingTableGrammar(t *testing.T) {
	ns, err := NamespaceFor("foo-1")
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range ns {
		digit := r >= '0' && r <= '9'
		if (r < 'a' || r > 'z') && r != '_' && !digit {
			t.Fatalf("namespace %q holds %q, which trackingTable refuses", ns, r)
		}
		if digit && i == 0 {
			t.Fatalf("namespace %q starts with a digit", ns)
		}
	}
	// The longest namespace the name grammar can produce is ext_ plus a
	// 32-character name; its tracking table must still fit the same budget.
	longest, err := NamespaceFor(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	if n := len("schema_migrations_") + len(longest); n > 63 {
		t.Fatalf("the longest tracking table is %d bytes, over PostgreSQL's 63", n)
	}
}

func TestTrackingTableRefusesUnspellableNamespaces(t *testing.T) {
	// The namespace is interpolated into DDL and cannot be a parameter, so
	// the grammar check runs before the connection is ever touched — which
	// is why a nil conn is safe here and is itself the assertion that no
	// refused namespace reaches the database.
	for _, tc := range []struct{ ns, wantErr string }{
		{ns: "Core", wantErr: "want lower-case letters"},
		{ns: "ext-foo", wantErr: "want lower-case letters"},
		{ns: "ext foo", wantErr: "want lower-case letters"},
		{ns: "1ext", wantErr: "cannot start with a digit"},
		{ns: "", wantErr: "empty namespace"},
	} {
		if _, err := trackingTable(t.Context(), nil, tc.ns); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("trackingTable(%q) err = %v, want %q", tc.ns, err, tc.wantErr)
		}
	}
}

// A version applied under a different name is a renumber, and it must stop the
// run rather than read as done.
//
// The failure it prevents is silent: the database recorded 0209 against a
// migration that has since become 0211, so the 0209 now on disk — an unrelated
// migration — would be skipped as already applied and never create anything it
// declares. Nothing later reports that; the first symptom is a missing
// relation at runtime, long after the deploy that caused it.
func TestALedgerRowNamingADifferentMigrationStopsTheRun(t *testing.T) {
	t.Parallel()
	applied := map[string]appliedRow{"0209": {name: "drop_workspace_identity_columns"}}

	err := assertLedgerMatches("core", applied, Migration{Version: "0209", Name: "contact_record_page_v2"})
	if err == nil {
		t.Fatal("a version applied under another name read as a match; the migration on disk would be skipped as done")
	}
	for _, want := range []string{"drop_workspace_identity_columns", "contact_record_page_v2", "dev-fresh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name both migrations and the repair; %q is missing from %q", want, err)
		}
	}
}

// The same version under the same name is an ordinary already-applied
// migration, and an unrecorded version is ordinary work to do. Neither may
// trip the guard, or every run would refuse.
func TestALedgerRowMatchingItsMigrationIsNotARenumber(t *testing.T) {
	t.Parallel()
	applied := map[string]appliedRow{"0209": {name: "contact_record_page_v2"}}

	if err := assertLedgerMatches("core", applied, Migration{Version: "0209", Name: "contact_record_page_v2"}); err != nil {
		t.Errorf("an exact match refused: %v", err)
	}
	if err := assertLedgerMatches("core", applied, Migration{Version: "0210", Name: "consumer_mail_create_grant"}); err != nil {
		t.Errorf("an unapplied version refused: %v", err)
	}
}

// Down reads the digest Up records, which nothing did before: the column held
// the evidence and no code acted on it (#2141).
//
// The down half is the sharper one. Up skipping an edited migration leaves a
// database missing whatever the edit added — bad, and visible later as an
// absent object. Down running the CURRENT rollback against a schema the OLD
// up-migration built is a schema CHANGE made on a false premise: it drops what
// this version's down names, which is not what that database has, and then
// deletes the row that was the only record of what it did have.
func TestARevertRefusesContentTheDatabaseNeverApplied(t *testing.T) {
	t.Parallel()
	was := Migration{Version: "0209", Name: "add_thing", UpSQL: "CREATE TABLE thing ()", DownSQL: "DROP TABLE thing"}
	stamped := Digest(was)
	applied := map[string]appliedRow{"0209": {name: was.Name, digest: &stamped}}

	// The same content is an ordinary revert.
	if err := assertContentMatches("core", applied, was); err != nil {
		t.Errorf("a revert of the content that was applied refused: %v", err)
	}

	// An edited DOWN is the case with teeth: the up half is identical, so the
	// database looks current by every other measure, and this rollback would
	// drop a different object from the one that exists.
	edited := was
	edited.DownSQL = "DROP TABLE thing CASCADE"
	if err := assertContentMatches("core", applied, edited); err == nil {
		t.Error("a revert whose down SQL was edited after it was applied was admitted — it would drop " +
			"what the source names rather than what the database has")
	}

	// An edited UP counts too. The digest covers both halves because a binary
	// whose up differs built a different schema, whatever its down says.
	edited = was
	edited.UpSQL = "CREATE TABLE thing (id int)"
	if err := assertContentMatches("core", applied, edited); err == nil {
		t.Error("a revert whose up SQL was edited after it was applied was admitted")
	}
}

// A row written before the digest column existed records no fingerprint, and
// that is a permanent answer rather than a gap to fill.
//
// Refusing on NULL would strand every installation that migrated before the
// column landed, with no way forward. Back-filling one would stamp a
// fingerprint over content nobody can recover — the exact divergence the column
// exists to expose.
func TestARevertOfAnUnverifiableRowIsAdmitted(t *testing.T) {
	t.Parallel()
	m := Migration{Version: "0209", Name: "add_thing", UpSQL: "CREATE TABLE thing ()", DownSQL: "DROP TABLE thing"}
	applied := map[string]appliedRow{"0209": {name: m.Name, digest: nil}}

	edited := m
	edited.DownSQL = "DROP TABLE thing CASCADE"
	if err := assertContentMatches("core", applied, edited); err != nil {
		t.Errorf("a revert of a row with no recorded digest refused: %v — unverifiable is not "+
			"the same as mismatched, and treating it as one strands every database that migrated "+
			"before the column existed", err)
	}
}

// A database that applied content named in supersededDigests reached the schema
// the current source builds, so it migrates on rather than stranding.
//
// The refusal case is what gives this one teeth: an unrelated mismatch on the
// SAME version must still be refused, or the entry would read as "stop checking
// this migration" rather than "this one earlier byte sequence is equivalent".
func TestSupersededContentIsAdmittedAndNothingElseIs(t *testing.T) {
	t.Parallel()
	const version = "1789122755"
	m := Migration{
		Version: version,
		Name:    "the_record_type_is_called_company",
		UpSQL:   readMigration(t, version+"_the_record_type_is_called_company.up.sql"),
		DownSQL: readMigration(t, version+"_the_record_type_is_called_company.down.sql"),
	}

	// The digest the broken content recorded on every database that applied it.
	admitted := equivalentContent["core"][version][0].applied
	if err := assertContentMatches("core", map[string]appliedRow{version: {name: m.Name, digest: &admitted}}, m); err != nil {
		t.Errorf("a database holding applied-but-equivalent content refused: %v — it reached the "+
			"schema this source builds and has nowhere else to go", err)
	}

	// Some OTHER applied content on the same version is still a mismatch.
	stranger := "bb" + strings.Repeat("0", 62)
	if err := assertContentMatches("core", map[string]appliedRow{version: {name: m.Name, digest: &stranger}}, m); err == nil {
		t.Error("applied content that is not the recorded one was admitted — an entry excuses one " +
			"known byte sequence, not every database on this version")
	}

	// And the entry must expire when the SOURCE changes again. Without this the
	// first excused edit would excuse every later one nobody checked.
	edited := m
	edited.UpSQL += "\n-- a further edit, which is a new claim\n"
	if err := assertContentMatches("core", map[string]appliedRow{version: {name: m.Name, digest: &admitted}}, edited); err == nil {
		t.Error("a FURTHER edit to this migration was admitted because the database held the " +
			"equivalent content — an entry names one source, not permission to keep editing")
	}

	// The allowance is per namespace, so custom's same version is unaffected.
	if err := assertContentMatches("custom", map[string]appliedRow{version: {name: m.Name, digest: &admitted}}, m); err == nil {
		t.Error("a core equivalence admitted the same version in the custom namespace")
	}
}

// Every entry must describe the migrations that actually ship: its source half
// must equal what the file hashes to today, and its applied half must differ
// from that. A stale source half silently stops excusing anything, which is the
// failure nobody would notice until a deployed database refused to migrate.
//
// The APPLIED half cannot be checked from this tree, and nothing here pretends
// to. It names bytes that no longer exist in the working tree — that is what
// makes it the applied half — so what it describes survives in version control
// and in the ledgers of the databases it names. A wrong one fails safe: it
// matches no ledger, those databases keep getting the ordinary refusal, and no
// unchecked content is ever let through, because a typo does not find a second
// preimage of a SHA-256. Verify an applied digest when you ADD an entry, against
// the content as committed; this test cannot do it for you later.
func TestEveryEquivalenceMatchesTheMigrationThatShips(t *testing.T) {
	t.Parallel()
	for namespace, versions := range equivalentContent {
		shipped := map[string]Migration{}
		for _, m := range loadNamespaceMigrations(t, namespace) {
			shipped[m.Version] = m
		}
		for version, entries := range versions {
			m, ok := shipped[version]
			if !ok {
				t.Errorf("%s %s: no such migration ships; the entry names nothing", namespace, version)
				continue
			}
			if len(entries) == 0 {
				t.Errorf("%s %s: an empty list excuses nothing; remove the entry", namespace, version)
			}
			for _, eq := range entries {
				if eq.source != Digest(m) {
					t.Errorf("%s %s: the entry's source digest is %s but the migration now hashes to "+
						"%s — the file changed again, so this equivalence describes content that no "+
						"longer ships and admits nobody", namespace, version, eq.source, Digest(m))
				}
				if eq.applied == eq.source {
					t.Errorf("%s %s: applied and source digests are equal, so the entry excuses "+
						"nothing the plain comparison did not already admit", namespace, version)
				}
			}
		}
	}
}

// loadNamespaceMigrations reads one namespace's migrations off disk, so the test
// above compares against what ships rather than a copy of it.
func loadNamespaceMigrations(t *testing.T, namespace string) []Migration {
	t.Helper()
	root := filepath.Join("..", "..", "..", "migrations")
	migrations, err := Load(os.DirFS(root), namespace)
	if err != nil {
		t.Fatalf("loading the %s migrations: %v", namespace, err)
	}
	return migrations
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "core", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

// A version this database never applied has no content to disagree about.
func TestARevertOfAnUnappliedVersionHasNothingToCompare(t *testing.T) {
	t.Parallel()
	m := Migration{Version: "0210", Name: "other", UpSQL: "SELECT 1", DownSQL: "SELECT 1"}
	if err := assertContentMatches("core", map[string]appliedRow{}, m); err != nil {
		t.Errorf("an unapplied version refused: %v", err)
	}
}
