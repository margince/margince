// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How the report engine refuses a plan it cannot run, and what shape an empty
// result arrives in. Both were reported as MCP defects: `run_report`
// internal-errored on exactly the vocabulary mistakes its own description
// promises to police, and answered `"rows": null` where every other list-shaped
// tool answers `[]`.
//
// No database: the vocabulary check and the row normalization both happen before
// and around the query, which is what makes them testable here at all.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestAnUnknownReportFieldIsAClassifiedCallerFault(t *testing.T) {
	// The verdict lives on the error, not in a transport helper: the MCP surface
	// reaches this engine through run_report and runs no HTTP helper, so a verdict
	// kept there would reach an agent as "the tool failed for an internal reason;
	// retry" — after the engine had already settled the call.
	fault, ok := httperr.Classify(&FieldNotAllowedError{Field: "nonsense"})
	if !ok {
		t.Fatal("FieldNotAllowedError is classified by nothing, so the tool surface reports it as an " +
			"internal fault and tells the agent to retry a call that can never succeed")
	}
	if fault.Transient() {
		t.Error("a vocabulary mistake is reported as transient; waiting cannot make `nonsense` a valid field")
	}
	if fault.Code != "report_field_not_allowed" {
		t.Errorf("code = %q, want the code writeReportError already puts on the REST wire", fault.Code)
	}
	// The rejected token has to survive into the message: it is what locates the
	// mistake among three arguments that could have carried it.
	if !strings.Contains(fault.Detail, "nonsense") {
		t.Errorf("the refusal does not quote the rejected name: %q", fault.Detail)
	}
}

func TestEveryVocabularyDoorRefusesAsACallerFault(t *testing.T) {
	// One door being classified says nothing about the others, so each is entered
	// here: a plan reaches the vocabulary through group_by, through an aggregate's
	// field, and through its function name — and a plan that selects nothing at all
	// reaches none of them and needs its own answer.
	spec := reportSpec{
		dimensions: map[string]string{"stage": "t.stage_id"},
		measures:   map[string]string{"amount": "t.amount_minor"},
	}
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"group_by", func() error {
			_, _, err := buildSelectList(spec, []string{"nonsense"}, nil)
			return err
		}},
		{"aggregates[].field", func() error {
			_, _, err := aggregateSelect(spec, reportAggregate{Fn: "sum", Field: "nonsense"})
			return err
		}},
		{"aggregates[].fn", func() error {
			_, _, err := aggregateSelect(spec, reportAggregate{Fn: "nonsense"})
			return err
		}},
		{"a plan selecting nothing", func() error {
			_, _, err := buildSelectList(spec, nil, nil)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("an out-of-vocabulary name was accepted")
			}
			fault, ok := httperr.Classify(err)
			if !ok {
				t.Fatalf("%v is classified by nothing", err)
			}
			if fault.Transient() {
				t.Errorf("%v is reported as transient", err)
			}
		})
	}
}

func TestAReportWithNoRowsAnswersAnEmptyArray(t *testing.T) {
	// reportOutcome.Rows is marshalled straight through on the tool surface, so
	// "matched nothing" has to already BE an array by the time it gets there —
	// there is no transport step left to normalize it.
	outcome := reportOutcome{Rows: scanned(t)}
	encoded, err := json.Marshal(map[string]any{"rows": outcome.Rows})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(encoded); got != `{"rows":[]}` {
		t.Errorf("an empty report marshals to %s, want {\"rows\":[]} — a model reads null as "+
			"\"unknown\" where an empty array says \"none matched\"", got)
	}
}

// scanned returns what scanReportRows produces for a result set with no rows.
// The normalization under test is the slice's initial value, so an empty result
// is exactly the case that exercises it.
func scanned(t *testing.T) []map[string]any {
	t.Helper()
	rows, err := scanReportRows(emptyRows{}, []string{"stage"})
	if err != nil {
		t.Fatalf("scanReportRows over an empty result: %v", err)
	}
	return rows
}

// emptyRows is a pgx.Rows that yielded nothing and failed at nothing — a query
// whose predicate matched no row. Only Next and Err are reached; the rest of the
// interface panics rather than returning a plausible zero, so a future scan step
// that starts calling them fails loudly here instead of silently agreeing with
// whatever this stub made up.
type emptyRows struct{}

func (emptyRows) Next() bool { return false }
func (emptyRows) Err() error { return nil }
func (emptyRows) Close()     {}
func (emptyRows) Conn() *pgx.Conn {
	panic("emptyRows: Conn is not part of the row scan")
}

func (emptyRows) CommandTag() pgconn.CommandTag {
	panic("emptyRows: CommandTag is not part of the row scan")
}

func (emptyRows) FieldDescriptions() []pgconn.FieldDescription {
	panic("emptyRows: FieldDescriptions is not part of the row scan")
}

func (emptyRows) Scan(...any) error {
	panic("emptyRows: Scan is unreachable — Next reported no rows")
}

func (emptyRows) Values() ([]any, error) {
	panic("emptyRows: Values is unreachable — Next reported no rows")
}

func (emptyRows) RawValues() [][]byte {
	panic("emptyRows: RawValues is not part of the row scan")
}

var _ pgx.Rows = emptyRows{}

// TestTwoFallbackAliasesKeepTheirOwnColumnNames holds the property that makes
// quoteIdent's shared "result" fallback safe.
//
// Two aggregates whose aliases are both outside the identifier shape select
// into the SAME SQL column name. That is only harmless because the SQL alias is
// never read back: the caller-facing names travel separately and the row map is
// built from those by position. If a future change ever keys rows by the SQL
// alias instead, this test is what fails — and the failure is a caller reading
// one measure's number under another measure's name.
func TestTwoFallbackAliasesKeepTheirOwnColumnNames(t *testing.T) {
	t.Parallel()

	spec := prebuiltReports["deals-by-stage"]

	firstName, firstSelect, err := aggregateSelect(spec, reportAggregate{Fn: aggFnCount, As: "total count"})
	if err != nil {
		t.Fatalf("a free-form alias was refused: %v", err)
	}
	secondName, secondSelect, err := aggregateSelect(spec, reportAggregate{Fn: aggFnCount, As: "1st try"})
	if err != nil {
		t.Fatalf("a free-form alias was refused: %v", err)
	}

	if firstName == secondName {
		t.Errorf("both aggregates report the caller-facing name %q — the row map is "+
			"keyed by these, so one measure would overwrite the other", firstName)
	}
	if firstName != "total count" || secondName != "1st try" {
		t.Errorf("caller-facing names were rewritten (%q, %q) — a caller reads its own "+
			"alias back, not the SQL identifier", firstName, secondName)
	}
	if !strings.Contains(firstSelect, "AS result") || !strings.Contains(secondSelect, "AS result") {
		t.Errorf("expected both selects to fall back to the fixed literal, got %q and %q "+
			"— an alias outside the identifier shape must never reach the SQL text",
			firstSelect, secondSelect)
	}

	// The admitting case: a well-formed alias rides into the SQL unchanged, so
	// the fallback above is reached only by names that need it.
	_, wellFormed, err := aggregateSelect(spec, reportAggregate{Fn: aggFnCount, As: "my_own_label"})
	if err != nil {
		t.Fatalf("a well-formed alias was refused: %v", err)
	}
	if !strings.Contains(wellFormed, "AS my_own_label") {
		t.Errorf("a well-formed alias did not reach the SQL text: %q", wellFormed)
	}
}
