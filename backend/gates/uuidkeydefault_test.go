// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every uuid `id` column defaults to uuidv7().
//
// ids.NewV7 is the only id-minting function in the tree. Data-model 1.1
// mandates v7 because ids minted by one process sort by creation order, which
// keeps B-tree inserts append-mostly. Several tables key their pagination on
// (created_at DESC, id DESC), and a random id there scatters inserts across
// the whole index.
//
// The default is what makes the rule unconditional. A rule held by the call
// sites lasts until a writer, an import path or a fixture does not know it.
//
// The corpus derives from the head catalog, so a new table arrives failing
// until it carries the default or a waiver.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// uuidV7Default is the whole default a key carries, not a substring of one.
const uuidV7Default = "uuidv7()"

// defaultMintsV7 reports whether this default mints a v7 for every insert.
//
// The whole default has to be the call. `coalesce(gen_random_uuid(), uuidv7())`
// names uuidv7 and never returns one, since coalesce takes the first non-null,
// so a containment test would admit a column that mints v4.
func defaultMintsV7(def string) bool {
	return strings.TrimSpace(def) == uuidV7Default
}

// uuidKeyLine reads a catalog line for a uuid `id` column, capturing its table
// and its default.
var uuidKeyLine = regexp.MustCompile(`^public\.(\w+)\.id uuid NOT NULL gen=\S+ def=(.*)$`)

// mintsItsOwnKey records each table whose id has no default, with why. It is
// what makes a new table fail: an unlisted one neither carries the rule nor
// records a reason for standing outside it.
var mintsItsOwnKey = gatekit.Waive(map[string]string{})

// uuidKeyDefaults derives the corpus, mapping a table to its id default.
func uuidKeyDefaults(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	defaults := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := uuidKeyLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			defaults[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return defaults
}

func TestEveryUUIDKeyDefaultsToV7(t *testing.T) {
	t.Parallel()
	defer mintsItsOwnKey.AssertAllMatched(t)
	defaults := uuidKeyDefaults(t)
	if len(defaults) < 200 {
		t.Fatalf("the catalog yields %d uuid id columns; this gate would judge a fraction of "+
			"the tree, so the reader has stopped matching the catalog's line shape", len(defaults))
	}
	tables := make([]string, 0, len(defaults))
	for table := range defaults {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		if defaultMintsV7(defaults[table]) || mintsItsOwnKey.Waived(t, table) {
			continue
		}
		t.Errorf("%s.id has no uuidv7() default (def=%s), so a caller that passes nothing, or "+
			"passes a v4, is not refused. Add the default in a migration, or waive the table in "+
			"mintsItsOwnKey with the reason its key comes from elsewhere.", table, defaults[table])
	}
}

// The reader is held to the shapes it must see. The corpus is a line pattern,
// and a pattern that stops matching reports a clean census.
func TestTheUUIDKeyReaderSeesEachShape(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		line    string
		want    bool
		defText string
	}{
		{"public.deal.id uuid NOT NULL gen=- def=uuidv7()", true, "uuidv7()"},
		{"public.mail_draft.id uuid NOT NULL gen=- def=-", true, "-"},
		{"public.deal.name text NOT NULL gen=- def=-", false, ""},
		{"public.river_job.id bigint NOT NULL gen=- def=nextval('x')", false, ""},
		{"public.contact.contact_id uuid NOT NULL gen=- def=-", false, ""},
		{
			"public.x.id uuid NOT NULL gen=- def=coalesce(gen_random_uuid(), uuidv7())", true,
			"coalesce(gen_random_uuid(), uuidv7())",
		},
	} {
		m := uuidKeyLine.FindStringSubmatch(c.line)
		if (m != nil) != c.want {
			t.Errorf("the uuid key reader matched %q = %v, want %v", c.line, m != nil, c.want)
			continue
		}
		if m != nil && strings.TrimSpace(m[2]) != c.defText {
			t.Errorf("the reader read %q's default as %q, want %q", c.line, m[2], c.defText)
		}
	}
}

// A default that merely NAMES uuidv7 does not mint one, which is the reading
// this gate has to refuse.
func TestOnlyAWholeV7CallMintsTheKey(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		def  string
		want bool
	}{
		{"uuidv7()", true},
		{" uuidv7() ", true},
		{"-", false},
		{"gen_random_uuid()", false},
		{"coalesce(gen_random_uuid(), uuidv7())", false},
		{"coalesce(uuidv7(), gen_random_uuid())", false},
	} {
		if got := defaultMintsV7(c.def); got != c.want {
			t.Errorf("defaultMintsV7(%q) = %v, want %v", c.def, got, c.want)
		}
	}
}
