// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The Art. 15 export over the columns that record what a value REPLACED.
//
// A newer dated statement by the contact — a mail signature, a business card —
// overwrites what the record held and keeps the old value on the row. That
// makes the row hold two assertions about the subject, plus the date the record
// believes each was stated. Exporting only the current one hands the subject a
// package that is true and incomplete: it says their title is X, while the
// installation also holds that it said Y until a message dated Z replaced it.
//
// The census at the bottom is the part that survives the next schema change. A
// column added to either table and left out of the SELECT is invisible in
// production and in every behavioural test above it, because the export simply
// never mentions it — nothing fails, and the subject is quietly owed something.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The subject's superseded history. The replaced values differ from the live
// ones so an assertion can name which of the two it found.
const (
	livedTitle    = "Head of Partnerships"
	replacedTitle = "Partnerships Lead"
	replacedPhone = "+493033333333"
)

// observedAt and replacedObservedAt are the dates the RECORD believes the
// subject stated each value — fixed literals, because the assertions ask only
// whether the date reached the export.
var (
	observedAt         = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	replacedObservedAt = time.Date(2025, 6, 1, 9, 0, 0, 0, time.UTC)
)

// seedSupersededHistory gives the subject a profile field that replaced an
// earlier answer, and a phone row that replaced an earlier number.
//
// The phone pair is seeded as two rows joined by superseded_phone_id, which is
// how the writer records a replacement: the old row is archived rather than
// deleted, and the new one points back at it.
func seedSupersededHistory(ctx context.Context, t *testing.T, owner *pgx.Conn, person ids.PersonID) {
	t.Helper()
	if _, err := owner.Exec(ctx, `
		INSERT INTO person_profile_field
		  (person_id, field, value, evidence_snippet, source_ref, source, captured_by,
		   observed_at, superseded_value, superseded_captured_by, superseded_observed_at)
		VALUES ($1, 'title', $2, 'signature block', 'activity:test', 'signature', 'agent:enrich',
		        $3, $4, 'user:test', $5)`,
		person, livedTitle, observedAt, replacedTitle, replacedObservedAt); err != nil {
		t.Fatal(err)
	}

	var replaced ids.UUID
	if err := owner.QueryRow(ctx, `
		INSERT INTO person_phone (person_id, phone, source, captured_by, observed_at, archived_at)
		VALUES ($1, $2, 'manual', 'user:test', $3, $4)
		RETURNING id`,
		person, ident(person, replacedPhone), replacedObservedAt, retiredAt).Scan(&replaced); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE person_phone SET superseded_phone_id = $1, observed_at = $2
		 WHERE person_id = $3 AND phone = $4`,
		replaced, observedAt, person, ident(person, livePhone)); err != nil {
		t.Fatal(err)
	}
}

// TestTheExportCarriesTheValueANewerStatementReplaced asserts the profile-field
// section hands over both assertions and both dates.
func TestTheExportCarriesTheValueANewerStatementReplaced(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedSupersededHistory(e.ctx, t, e.owner, e.person)

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	var title map[string]any
	for _, row := range pkg.EnrichedFields {
		if row["field"] == "title" {
			title = row
		}
	}
	if title == nil {
		t.Fatalf("the enriched title is missing from the export: %v", pkg.EnrichedFields)
	}

	if got := title["value"]; got != livedTitle {
		t.Errorf("current value = %v, want %q", got, livedTitle)
	}
	// The replaced value is the assertion the export used to drop. A subject
	// reading only the current one cannot tell that this installation also
	// holds what it said before, or that a colleague was the one who said it.
	if got := title["superseded_value"]; got != replacedTitle {
		t.Errorf("superseded_value = %v, want %q — Art. 15 owes the value still held on the row", got, replacedTitle)
	}
	if got := title["superseded_captured_by"]; got != "user:test" {
		t.Errorf("superseded_captured_by = %v, want the colleague who wrote the replaced value", got)
	}
	for _, field := range []struct {
		key  string
		want time.Time
	}{
		{"observed_at", observedAt},
		{"superseded_observed_at", replacedObservedAt},
	} {
		stamp, ok := title[field.key].(time.Time)
		if !ok {
			t.Errorf("%s is absent from the export: %v", field.key, title)
			continue
		}
		if !stamp.Equal(field.want) {
			t.Errorf("%s = %v, want %v", field.key, stamp, field.want)
		}
	}
}

// TestTheExportNamesTheNumberANewerOneReplaced asserts the phone section
// resolves superseded_phone_id to the number it points at.
//
// The column holds a row id. Exporting it raw would satisfy a census and tell
// the subject nothing, so the obligation is the NUMBER.
func TestTheExportNamesTheNumberANewerOneReplaced(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedSupersededHistory(e.ctx, t, e.owner, e.person)

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	var live map[string]any
	for _, row := range pkg.Phones {
		if row["phone"] == ident(e.person, livePhone) {
			live = row
		}
	}
	if live == nil {
		t.Fatalf("the live phone is missing from the export: %v", pkg.Phones)
	}
	if got := live["replaced_phone"]; got != ident(e.person, replacedPhone) {
		t.Errorf("replaced_phone = %v, want %q — a row id would name nothing the subject can read", got, replacedPhone)
	}
	stamp, ok := live["replaced_observed_at"].(time.Time)
	if !ok {
		t.Fatalf("replaced_observed_at is absent: %v", live)
	}
	if !stamp.Equal(replacedObservedAt) {
		t.Errorf("replaced_observed_at = %v, want %v", stamp, replacedObservedAt)
	}
}

// A phone row that replaced nothing must export the pair as absent rather than
// borrowing another row's. The LEFT JOIN is what makes that true, and a plain
// JOIN — the easy edit — would silently drop every number that replaced nothing,
// which is most of them.
func TestAPhoneThatReplacedNothingStillReachesTheExport(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedSupersededHistory(e.ctx, t, e.owner, e.person)

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	var replaced map[string]any
	for _, row := range pkg.Phones {
		if row["phone"] == ident(e.person, replacedPhone) {
			replaced = row
		}
	}
	if replaced == nil {
		t.Fatalf("the replaced phone is itself missing from the export — Art. 15 owes what is held: %v", pkg.Phones)
	}
	if got := replaced["replaced_phone"]; got != nil {
		t.Errorf("replaced_phone = %v on a row that replaced nothing, want nil", got)
	}
}

// TestEveryHeldColumnReachesTheExport is the census. It reads the live table
// definition and fails on a column no section projects.
//
// Derived from the database rather than from a list kept here, because a list
// is updated by whoever remembers it exists. The excused set is small and each
// entry says why it is not the subject's data; adding to it is a decision a
// reader can see, which is the whole difference between an omission and a
// judgement.
func TestEveryHeldColumnReachesTheExport(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedSupersededHistory(e.ctx, t, e.owner, e.person)

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	for _, subject := range []struct {
		table   string
		rows    []map[string]any
		excused map[string]string
	}{
		{
			table: "person_profile_field",
			rows:  pkg.EnrichedFields,
			excused: map[string]string{
				"id":         "the row's own key, not a fact about the subject",
				"person_id":  "the subject this whole package is about",
				"created_at": "updated_at is the one the section reports",
				"version":    "optimistic-locking counter; says how often the row was written, not what it says",
			},
		},
		{
			table: "person_phone",
			rows:  pkg.Phones,
			excused: map[string]string{
				"id":                  "the row's own key, not a fact about the subject",
				"person_id":           "the subject this whole package is about",
				"created_at":          "bookkeeping; observed_at is what the record believes",
				"updated_at":          "bookkeeping; observed_at is what the record believes",
				"superseded_phone_id": "exported resolved, as replaced_phone",
				"source":              "how it was captured, reported by the provenance section",
				"captured_by":         "who captured it, reported by the provenance section",
				"is_primary":          "a display choice this installation made, not the subject's data",
				"position":            "the order the numbers are listed in, a display choice",
				"version":             "optimistic-locking counter; says how often the row was written, not what it says",
			},
		},
	} {
		t.Run(subject.table, func(t *testing.T) {
			rows, err := e.owner.Query(e.ctx, `
				SELECT column_name FROM information_schema.columns
				 WHERE table_schema = 'public' AND table_name = $1`, subject.table)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()

			var columns []string
			for rows.Next() {
				var column string
				if err := rows.Scan(&column); err != nil {
					t.Fatal(err)
				}
				columns = append(columns, column)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			// A census that reads no columns reports PASS while proving
			// nothing, which is the one way this test could fail short.
			if len(columns) == 0 {
				t.Fatalf("read no columns for %s — the census has nothing to check", subject.table)
			}
			if len(subject.rows) == 0 {
				t.Fatalf("the export carries no %s rows, so no column can be seen in one", subject.table)
			}

			exported := subject.rows[0]
			for _, column := range columns {
				if reason, ok := subject.excused[column]; ok {
					if reason == "" {
						t.Errorf("%s.%s is excused without a reason", subject.table, column)
					}
					continue
				}
				if _, ok := exported[column]; !ok {
					t.Errorf("%s.%s is held about the subject but reaches no export section — "+
						"add it to the SELECT, or excuse it with the reason it is not their data",
						subject.table, column)
				}
			}
		})
	}
}
