// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A row that was never changed carries the moment it was written in
// updated_at. A null there drops the row from a sort or a filter on the
// column. So the database gives the column a default and refuses a null.
//
// The corpus derives from the head catalog: each column named updated_at.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// updatedAtLine reads an updated_at column: its table and its definition.
var updatedAtLine = regexp.MustCompile(`^public\.(\w+)\.updated_at (.*)$`)

// houseUpdatedAt is the definition the catalog prints for the house shape.
const houseUpdatedAt = "timestamp with time zone NOT NULL gen=- def=now()"

// offShapeUpdatedAt answers each updated_at column of a catalog that is not
// the house shape, and how many updated_at columns the catalog has.
func offShapeUpdatedAt(catalog string) (off []string, seen int) {
	for line := range strings.SplitSeq(catalog, "\n") {
		m := updatedAtLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		seen++
		if m[2] != houseUpdatedAt {
			off = append(off, m[1]+": "+m[2])
		}
	}
	return off, seen
}

func TestEveryUpdatedAtIsNeverNullAndDefaultsToNow(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	off, seen := offShapeUpdatedAt(string(raw))
	// A short census would pass on a catalog whose line shape had moved.
	if seen < 100 {
		t.Fatalf("read %d updated_at columns from the catalog, and the schema has over a hundred", seen)
	}
	for _, column := range off {
		t.Errorf("updated_at on %s is not %q. Give it DEFAULT now() and NOT NULL; "+
			"a reader that asks whether a row changed compares it with created_at.", column, houseUpdatedAt)
	}
}

// The reader sees each way a column leaves the shape.
func TestTheUpdatedAtCensusSeesEachOffShapeColumn(t *testing.T) {
	t.Parallel()
	catalog := strings.Join([]string{
		"public.kept.updated_at " + houseUpdatedAt,
		"public.nullable.updated_at timestamp with time zone gen=- def=now()",
		"public.bare.updated_at timestamp with time zone NOT NULL gen=- def=-",
		"public.kept.created_at timestamp with time zone NOT NULL gen=- def=now()",
	}, "\n")
	off, seen := offShapeUpdatedAt(catalog)
	if seen != 3 || len(off) != 2 {
		t.Fatalf("read %d columns and found %d off shape (%v), want 3 and 2", seen, len(off), off)
	}
}
