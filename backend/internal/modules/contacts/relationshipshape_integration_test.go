// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The Go shape table and the database's shape CHECKs are one statement.
//
// Two answers to "which endpoints does this kind take" is exactly how the
// incomplete pair survived: `rel_employment_shape` and `rel_stakeholder_shape`
// required their own ends and nulled the deal, the project and the counterparty
// contact — and never the counterparty COMPANY, so a three-ended employment
// landed and nothing anywhere refused it.
//
// So the Go table is a declared mirror rather than a second opinion, and this
// fails in both directions: a kind whose REQUIRED ends the table gets wrong,
// and a kind in the table the database has never heard of. It reads
// `pg_get_constraintdef` off the LIVE schema rather than parsing the
// migrations, because the constraints have been dropped and re-added three
// times and the installed definition is the only one that governs a write.
//
// WHAT IT COMPARES IS THE REQUIRED SET, and the asymmetry is deliberate rather
// than an omission. On the forbidden half the Go table is STRICTER than the
// database today — that is the whole defect, and closing it in SQL needs a
// migration and a check-and-report pass over rows that already exist, which is
// this issue's other half. Comparing the forbidden sets too would fail on the
// very gap this change routes around, so it would have to be waived per kind,
// and a waiver naming the two kinds is a worse record of the gap than the
// paragraph above.

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// notNullIn / nullIn read a constraint definition for the columns it requires
// and the ones it forbids. The definition arrives normalized by Postgres —
// `((kind <> 'employment'::text) OR ((contact_id IS NOT NULL) AND …))` — so the
// two patterns are exact rather than approximate.
var (
	notNullIn = regexp.MustCompile(`\(([a-z_]+) IS NOT NULL\)`)
	nullIn    = regexp.MustCompile(`\(([a-z_]+) IS NULL\)`)
	kindsIn   = regexp.MustCompile(`'([a-z_]+)'::text`)
)

// shapeConstraints reads every rel_%_shape CHECK as the kinds it governs and
// the endpoint columns it requires.
func shapeConstraints(ctx context.Context, t *testing.T, tx pgx.Tx) map[string][]string {
	t.Helper()
	rows, err := tx.Query(ctx, `
		SELECT conname, pg_get_constraintdef(oid)
		  FROM pg_constraint
		 WHERE conrelid = 'relationship'::regclass AND conname LIKE 'rel\_%\_shape'`)
	if err != nil {
		t.Fatalf("reading the relationship shape constraints: %v", err)
	}
	defer rows.Close()
	required := map[string][]string{}
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			t.Fatalf("scanning a shape constraint: %v", err)
		}
		// The kinds the arm governs, and the columns it demands of them. A
		// constraint naming several kinds (the three partner kinds share one)
		// states one shape for all of them.
		var columns []string
		for _, m := range notNullIn.FindAllStringSubmatch(def, -1) {
			columns = append(columns, m[1])
		}
		sort.Strings(columns)
		for _, m := range kindsIn.FindAllStringSubmatch(def, -1) {
			required[m[1]] = columns
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the shape constraints: %v", err)
	}
	return required
}

// endpointColumnsIn reads every column the shape constraints speak about, which
// is the schema's own answer to "what is an endpoint column".
func endpointColumnsIn(ctx context.Context, t *testing.T, tx pgx.Tx) []string {
	t.Helper()
	rows, err := tx.Query(ctx, `
		SELECT pg_get_constraintdef(oid)
		  FROM pg_constraint
		 WHERE conrelid = 'relationship'::regclass AND conname LIKE 'rel\_%\_shape'`)
	if err != nil {
		t.Fatalf("reading the shape constraints: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			t.Fatalf("scanning a shape constraint: %v", err)
		}
		for _, pattern := range []*regexp.Regexp{notNullIn, nullIn} {
			for _, m := range pattern.FindAllStringSubmatch(def, -1) {
				seen[m[1]] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the shape constraints: %v", err)
	}
	columns := make([]string, 0, len(seen))
	for column := range seen {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

// The Go list of endpoint columns is the schema's, which is what makes
// "everything a kind does not require is refused" a closed statement. A column
// added to the table and not to that list would be an end no kind declares and
// nothing refuses — the same shape as the gap this change closes, one level up.
func TestTheEndpointColumnsAreTheSchemasOwn(t *testing.T) {
	e := setupDedupe(t)
	ctx := context.Background()
	var fromDB []string
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		fromDB = endpointColumnsIn(ctx, t, tx)
		return nil
	}); err != nil {
		t.Fatalf("opening a read of the schema: %v", err)
	}
	got := append([]string(nil), relationshipEndpointColumns...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(fromDB, ",") {
		t.Errorf("the Go endpoint columns are %v and the shape constraints speak about %v — a column "+
			"in the schema and not in Go is an end no kind declares and the writer never refuses",
			got, fromDB)
	}
}

func TestTheEndpointShapesAgreeWithTheDatabase(t *testing.T) {
	e := setupDedupe(t)
	ctx := context.Background()
	var fromDB map[string][]string
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		fromDB = shapeConstraints(ctx, t, tx)
		return nil
	}); err != nil {
		t.Fatalf("opening a read of the schema: %v", err)
	}
	// Fail closed: a query that matched no constraint would agree with an empty
	// Go table and report PASS over every kind.
	if len(fromDB) < len(relationshipShapes) {
		t.Fatalf("read %d kinds out of the schema's shape constraints against %d in the Go table — "+
			"the constraint query is reading less than the database declares, which agrees with "+
			"anything", len(fromDB), len(relationshipShapes))
	}

	for kind, shape := range relationshipShapes {
		want, governed := fromDB[kind]
		if !governed {
			t.Errorf("the Go table gives %q a shape and no rel_*_shape CHECK governs it — either the "+
				"kind is not in the schema, or its constraint was dropped and the database now "+
				"admits any endpoints for it", kind)
			continue
		}
		got := append([]string(nil), shape.requires...)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%q requires %v in Go and %v in the database — one of them is wrong about which "+
				"records this edge hangs on, and the writer refuses by the Go answer while the row "+
				"is admitted by the other", kind, got, want)
		}
	}
	for kind := range fromDB {
		if _, known := relationshipShapes[kind]; !known {
			t.Errorf("the schema constrains %q and the Go table has no shape for it, so the writer "+
				"refuses it outright — add it to relationshipShapes", kind)
		}
	}
}

// The defect itself, against a real write: the two kinds whose CHECK never
// nulled the counterparty company.
//
// Through the module's own writer rather than an INSERT, because what was wrong
// was the WRITE PATH: a hand-rolled insert would prove only that the CHECK
// still admits the row, which it does — that half is a separate change.
// seedShapeCompany is one company through the module's own writer, for the two
// ends this suite needs to tell apart.
func (e *dedupeEnv) seedShapeCompany(ctx context.Context, t *testing.T, name string) ids.CompanyID {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("seed company %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func TestAnEdgeCannotCarryAnEndpointItsKindDoesNotTake(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	employer := e.seedShapeCompany(ctx, t, "Brandt GmbH")
	stranger := e.seedShapeCompany(ctx, t, "Globex")

	// The control: the same edge without the stray end is accepted, so the
	// refusal below is about the endpoint and not about the fixture.
	control := e.seedContact(ctx, t, "Ute Sommer", nil, nil)
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &control, CompanyID: &employer,
	}); err != nil {
		t.Fatalf("a two-ended employment was refused, so this fixture proves nothing: %v", err)
	}

	// A DIFFERENT contact for the three-ended write. Reusing the control's
	// would meet the one-live-employment rule first, and the run would go red
	// on that instead — which is a pass for the wrong reason, and how the
	// mutation check caught this fixture the first time.
	contact := e.seedContact(ctx, t, "Jonas Petersen", nil, nil)
	var shapeErr *RelationshipShapeError
	_, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &contact, CompanyID: &employer,
		CounterpartyCompanyID: &stranger,
	})
	if !errors.As(err, &shapeErr) {
		t.Errorf("an employment carrying a counterparty company answered %v, want a shape refusal — "+
			"the row lands, and every reader that names \"the other end\" walks the columns in a "+
			"fixed order and never mentions it", err)
	}
}
