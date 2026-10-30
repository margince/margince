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
// sites is held until a writer, an import path or a fixture does not know it.
// The list of tables missing it grew from five to twelve while the issue
// behind this gate was open.
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
		if strings.Contains(defaults[table], "uuidv7()") || mintsItsOwnKey.Waived(t, table) {
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
