// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// Every column shaped like a stored object's key is one the reap consults, or says why not.
//
// The reap deletes a provisional key only when none of its kind's declared
// columns names it, so a declaration that misses a column is the one bug that
// reaches live bytes: a key recorded there reads as unreferenced and its file
// is deleted. Read from the committed schema catalog, so a migration adding
// such a column reaches this census through the file its own gate regenerates.
//
// What it cannot see: a key held in a column whose name does not fit the
// shape, or inside a json document or a text body.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// storedObjectKeyName is the name shape of a column recording a stored
// object's key, measured from the columns every blob writer records into.
var storedObjectKeyName = regexp.MustCompile(`^(?:storage_key|source_ref|[a-z0-9_]+_object_key|[a-z0-9_]+_asset_ref)$`)

// keyShapedColumnsNotReferences are the key-shaped columns no reap
// declaration names, each with why that is safe.
//
// gatekit:fixture the reason each key-shaped column is not a reap reference — expected data about the schema.
var keyShapedColumnsNotReferences = map[string]string{
	"stored_object_intent.storage_key": "the ledger itself: it records the provisional key, it does not reference one",
	"contact.photo_object_key": "no writer stores a photo; TestTheSubjectPhotoIsNeverStoredWhereErasureCannotFollow " +
		"holds the column NULL, and the writer that lifts it declares the kind",
	"contact_profile_field.source_ref": "names the message or page a profile fact was read from, not an object in the store",
	"voice_corpus_source.source_ref":   "a voice source's natural key — an external id or a content hash — not an object in the store",
}

func TestEveryStoredObjectKeyColumnIsDeclaredToTheReap(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, ref := range compose.StoredObjectReferences() {
		for _, col := range ref.Columns {
			declared[col.Table+"."+col.Name] = true
		}
	}
	for _, problem := range judgeKeyColumns(keyShapedColumns(catalogRecords(t)), declared, keyShapedColumnsNotReferences) {
		t.Error(problem)
	}
}

// TestTheKeyColumnCensusCanFail plants an undeclared key column behind a
// wrapped record, a constraint whose NAME fits the shape, and a stale entry.
func TestTheKeyColumnCensusCanFail(t *testing.T) {
	t.Parallel()
	planted := strings.Join([]string{
		"public.upload acl=- rls=false force=false",
		"public.upload.blob_object_key text NOT NULL gen=- def=-",
		"public.upload.declared_asset_ref text gen=- def=-",
		"public.upload.upload_object_key UNIQUE (blob_object_key)",
		"public.upload_live_idx CREATE INDEX upload_live_idx ON public.upload USING btree (id) WHERE (CASE",
		"    WHEN true THEN 1",
		"END = 1)",
		"public.upload.wrapped_object_key text NOT NULL gen=- def=CASE",
		"    WHEN true THEN ''::text",
		"END",
		"public.upload.storage_key uuid NOT NULL gen=- def=-",
	}, "\n")
	columns := keyShapedColumns(catalogRecordsOf(planted))

	if want := []string{"upload.blob_object_key", "upload.declared_asset_ref", "upload.wrapped_object_key"}; !slices.Equal(columns, want) {
		t.Fatalf("the census read %v, want %v — it misreads a wrapped record, a constraint or a non-text column", columns, want)
	}
	problems := judgeKeyColumns(columns,
		map[string]bool{"upload.declared_asset_ref": true, "upload.retired_object_key": true},
		map[string]string{"upload.gone_object_key": "planted stale reason", "upload.wrapped_object_key": ""})
	if len(problems) != 4 {
		t.Errorf("judging the planted schema raised %d problems, want 4 — an undeclared column, one excused with "+
			"no reason, one declaration and one reason naming a column the schema lacks: %q", len(problems), problems)
	}
}

// keyShapedColumns answers the text columns whose name fits the key shape, as
// table.column. Records are matched whole: a constraint's or an index's tail
// can carry the `gen=`/`def=` a column record ends with.
func keyShapedColumns(records []string) []string {
	var out []string
	for _, record := range records {
		if catalogIndex.MatchString(record) || catalogTrigger.MatchString(record) ||
			catalogConstraint.MatchString(record) || catalogFunction.MatchString(record) {
			continue
		}
		m := catalogColumn.FindStringSubmatch(record)
		if m == nil || !storedObjectKeyName.MatchString(m[2]) {
			continue
		}
		if dataType := strings.TrimSuffix(m[3], " NOT NULL"); dataType == "text" || strings.HasPrefix(dataType, "character varying") {
			out = append(out, m[1]+"."+m[2])
		}
	}
	slices.Sort(out)
	return out
}

// judgeKeyColumns holds the schema, the declarations and the reasons to each
// other in every direction, so none of the three can drift alone.
func judgeKeyColumns(columns []string, declared map[string]bool, reasons map[string]string) []string {
	var problems []string
	inSchema := map[string]bool{}
	for _, column := range columns {
		inSchema[column] = true
		reason, excused := reasons[column]
		switch {
		case !declared[column] && excused && strings.TrimSpace(reason) == "":
			problems = append(problems, column+" is in keyShapedColumnsNotReferences with no reason: say why it "+
				"holds no stored object's key")
		case declared[column] && excused:
			problems = append(problems, column+" is declared to the reap and also excused in "+
				"keyShapedColumnsNotReferences: drop the excuse")
		case !declared[column] && !excused:
			problems = append(problems, column+" is shaped like a stored object's key and no StoredObjectReference "+
				"names it, so the reap would delete bytes it records. Add it to the owning module's declaration, "+
				"or, if it holds no object key, to keyShapedColumnsNotReferences with why")
		}
	}
	for column := range declared {
		if !inSchema[column] {
			problems = append(problems, column+" is declared to the reap but is no key-shaped text column in the "+
				"catalog: the reap's read would fail, or storedObjectKeyName has stopped matching a real reference")
		}
	}
	for column := range reasons {
		if !inSchema[column] {
			problems = append(problems, column+" is in keyShapedColumnsNotReferences but no key-shaped column has "+
				"that name any more: drop the entry")
		}
	}
	slices.Sort(problems)
	return problems
}
