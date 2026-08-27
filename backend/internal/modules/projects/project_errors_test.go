// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The schema's business rules reach a caller through this mapping, so the
// mapping is what decides whether a breach reads as "you broke rule X" or as
// an opaque server fault. The named-constraint list here is DERIVED from the
// migration rather than retyped: a rule added to the table and forgotten in
// the switch shows up as a test failure instead of a 500 on the one path
// nobody exercised.

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// headCatalog is the committed rendering of the schema every migration
// builds (migrations/testdata/head_catalog.txt) — the source of truth for
// which CHECK constraints the project table carries TODAY, amendments
// included. Reading the baseline alone would miss a CHECK a later migration
// added, which is exactly the rule most likely to have no message yet.
const headCatalog = "../../../migrations/testdata/head_catalog.txt"

// projectCheckLine finds the catalog rows that are CHECK constraints on the
// project table. An inline unnamed CHECK gets a generated name and is covered
// by the constraint net, which is exactly what the net is for.
var projectCheckLine = regexp.MustCompile(`(?m)^public\.project\.([a-z_]+) CHECK `)

func projectCheckConstraints(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(headCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	var names []string
	for _, m := range projectCheckLine.FindAllStringSubmatch(string(raw), -1) {
		names = append(names, m[1])
	}
	if len(names) == 0 {
		t.Fatal("found no CHECK constraints on project — the pattern no longer reads the catalog")
	}
	return names
}

// unreachableChecks are the table's CHECKs no request can violate, so a bespoke
// message for one would be a branch no caller can execute.
//
// A WAIVER, not a fixture: it exempts its subjects from this file's obligation,
// and gatekit.Waive is what makes that exemption say its cost, hold a floor on
// the reason's length, and fail once an entry stops matching live code.
//
// The bar for an entry is that NO WRITER CAN REACH IT — verified against the
// store, not inferred from the contract. The contract declaring `phase` an enum
// is NOT such a reason: httperr.Decode does not validate enums and this
// installation runs no request-validator middleware, so an unknown phase reaches
// the CHECK. That constraint has a real message now (ProjectPhaseError).
var unreachableChecks = gatekit.Waive(map[string]string{
	"project_visibility_check": "no writer names the visibility column — head " +
		"narrowed the CHECK to visibility = 'workspace', the contract exposes no " +
		"project visibility field, and nothing in the store sets one, so no " +
		"request can produce a row that violates it",
})

// Every CHECK the table carries must have a message of its own. Falling
// through to the constraint net is not a failure of correctness — it still
// answers 422 — but the net cannot name a field, so the caller is told only
// that some value is outside what its field accepts.
func TestEveryNamedProjectCheckHasItsOwnRefusal(t *testing.T) {
	defer unreachableChecks.AssertAllMatched(t)
	for _, constraint := range projectCheckConstraints(t) {
		if unreachableChecks.Waived(t, constraint) {
			continue
		}
		t.Run(constraint, func(t *testing.T) {
			err := projectCheckError(constraint, "")
			if err == nil {
				t.Fatalf("%s has no refusal of its own, so a caller breaking it is told only that a value is not allowed",
					constraint)
			}
			if strings.Contains(err.Error(), constraint) {
				t.Errorf("%s reports its own constraint name to the caller: %q", constraint, err.Error())
			}
			// The 422 is field-coded only if the refusal says which field.
			coded, ok := err.(interface {
				FieldFault() (string, string, string)
			})
			if !ok {
				t.Fatalf("%s answers %T, which names no field, so the 422 cannot say what to fix", constraint, err)
			}
			field, code, _ := coded.FieldFault()
			if field == "" || code == "" || strings.HasPrefix(field, "project_") {
				t.Errorf("%s answers field %q code %q, want a request field and a code a caller can act on", constraint, field, code)
			}
		})
	}
}

// keyRaceLost speaks only for the key index. Reading another constraint's
// violation as a lost race would retry a write that can only fail again, and
// swallow the real refusal while doing it.
func TestOnlyTheKeyIndexCountsAsALostRace(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		wantIt bool
	}{
		"its own index":      {&pgconn.PgError{Code: "23505", ConstraintName: "uq_project_key"}, true},
		"another index":      {&pgconn.PgError{Code: "23505", ConstraintName: "uq_rel_project_stakeholder"}, false},
		"not a uniqueness":   {&pgconn.PgError{Code: "23514", ConstraintName: "project_dates"}, false},
		"an ordinary error":  {errors.New("connection reset"), false},
		"nothing went wrong": {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			// A lost race means the mint loop takes the next number; anything
			// else is a refusal the caller has to be told about, so reading one
			// as the other either hides a real error behind a retry or turns a
			// retryable race into a 500.
			if got := keyRaceLost(tc.err); got != tc.wantIt {
				t.Fatalf("keyRaceLost(%v) = %v, want %v", tc.err, got, tc.wantIt)
			}
		})
	}
}

// No refusal on this surface may hand a client a schema identifier: an index
// or constraint name describes our tables and tells a caller nothing it can
// act on.
func TestProjectRefusalsKeepSchemaNamesOffTheWire(t *testing.T) {
	for _, err := range []error{
		&ProjectKeyShapeError{},
		&ClosedReasonRequiredError{},
		&ProjectPhaseError{},
		&ProjectDateRangeError{},
	} {
		for _, leak := range append(projectCheckConstraints(t), "uq_", "SQLSTATE") {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%T leaks %q to the caller: %q", err, leak, err.Error())
			}
		}
	}
}
