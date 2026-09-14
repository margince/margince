// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A uniqueness refusal on a relationship carries two facts, and callers need
// both: the SENTINEL, so the transport answers 409, and the CONSTRAINT, so a
// caller that races itself can tell which rule refused it and recover.
//
// It must carry them WITHOUT the driver error. A *pgconn.PgError anywhere in
// the chain makes httperr read the refusal as an infrastructure fault — the
// client's detail is blanked and every ordinary duplicate logs at ERROR — and
// the agent runner echoes err.Error() into its transcript, which would put
// SQLSTATE text into a model prompt.

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Derived from relationshipConflictDetails, not restated beside it: a rule
// added to the mapper without being covered here would otherwise be a rule
// nothing checks, and the mapper is the one list that has to be complete.
func TestRelationshipUniquenessRefusalKeepsBothTheSentinelAndTheConstraint(t *testing.T) {
	for constraint := range relationshipConflictDetails {
		t.Run(constraint, func(t *testing.T) {
			mapped := mapRelationshipConstraint(
				&pgconn.PgError{Code: "23505", ConstraintName: constraint}, "project_stakeholder")

			if !errors.Is(mapped, apperrors.ErrConflict) {
				t.Fatalf("mapped error %v does not carry ErrConflict — the transport would not answer 409", mapped)
			}
			var conflict *RelationshipConflictError
			if !errors.As(mapped, &conflict) {
				t.Fatal("the constraint was dropped — a caller cannot tell which rule refused it, so no recovery path can fire")
			}
			if conflict.Constraint != constraint {
				t.Fatalf("constraint = %q, want %q", conflict.Constraint, constraint)
			}
			// The driver error must NOT survive: httperr reads a PgError in the
			// chain as an infrastructure fault, and the agent runner would echo
			// its SQLSTATE text into a model prompt.
			var pgErr *pgconn.PgError
			if errors.As(mapped, &pgErr) {
				t.Fatalf("the driver error rode along: %v", mapped)
			}
			// httperr sends a sentinel's own text as the 409 detail, so the
			// message is client-facing: no SQLSTATE, and no index name either.
			for _, internal := range []string{"SQLSTATE", "23505", constraint, "uq_"} {
				if strings.Contains(mapped.Error(), internal) {
					t.Fatalf("the client-facing message carries the database internal %q: %q", internal, mapped.Error())
				}
			}
		})
	}
}

// Each rule refuses something different, so each has to SAY something
// different. The primary-employer index is keyed on the contact alone: its
// conflict is with another company, not with the pair the caller just named,
// and a message describing the wrong pair sends them hunting a row that does
// not exist.
func TestEachUniquenessRuleDescribesWhatItActuallyRefused(t *testing.T) {
	described := map[string]string{
		"uq_rel_current_primary_employer": "current primary employer",
		"uq_rel_deal_contact_role":        "role on the deal",
		employmentUnique:                  "already works at that company",
		projectStakeholderUnique:          "stakeholder on the project",
	}
	// The generic fallback below is a truthful refusal but a vague one, so no
	// rule may reach it by being forgotten here.
	for constraint := range relationshipConflictDetails {
		if _, covered := described[constraint]; !covered {
			t.Errorf("%s is mapped to a client message that nothing asserts", constraint)
		}
	}
	for constraint, want := range described {
		t.Run(constraint, func(t *testing.T) {
			got := (&RelationshipConflictError{Constraint: constraint}).Error()
			if !strings.Contains(got, want) {
				t.Fatalf("detail for %s = %q, want it to describe %q", constraint, got, want)
			}
			// The pair-shaped wording is the one that is false for the
			// contact-keyed rule, so no rule may fall back to it.
			if strings.Contains(got, "between these records") {
				t.Fatalf("%s reports a conflict between the named records, which its key does not establish: %q", constraint, got)
			}
		})
	}
}

// A refusal that is not a uniqueness violation must pass through untouched, so
// the recovery probe above cannot misfire on an unrelated failure.
func TestANonUniquenessErrorIsNotDressedAsAConflict(t *testing.T) {
	cause := errors.New("connection reset")
	if mapped := mapRelationshipConstraint(cause, "project_stakeholder"); !errors.Is(mapped, cause) {
		t.Fatalf("mapped = %v, want the original error preserved", mapped)
	}
	if errors.Is(mapRelationshipConstraint(cause, "project_stakeholder"), apperrors.ErrConflict) {
		t.Fatal("an unrelated failure was reported as a conflict")
	}
}

// AlreadyRecorded is what an idempotent writer asks before it treats a refusal
// as convergence, and the answer differs by which KEY refused.
//
// The rule keyed on the contact alone is the one that must answer no: its
// conflict is with a different company entirely, so the edge the caller offered
// did not land. An importer reading every 409 as "already there" reported a row
// it never wrote, and the contact kept an employer nobody recorded.
//
// The list is derived from the mapper's own vocabulary rather than typed twice,
// so a rule added there is answered here or names itself.
func TestOnlyATupleKeyedRefusalMeansTheEdgeIsAlreadyOnFile(t *testing.T) {
	contactKeyed := map[string]bool{"uq_rel_current_primary_employer": true}
	for constraint := range relationshipConflictDetails {
		t.Run(constraint, func(t *testing.T) {
			got := (&RelationshipConflictError{Constraint: constraint}).AlreadyRecorded()
			if want := !contactKeyed[constraint]; got != want {
				t.Fatalf("AlreadyRecorded() = %t for %s, want %t", got, constraint, want)
			}
		})
	}
	// uq_rel_works_with is answered by AlreadyRecorded but carries no
	// caller-facing sentence, so the loop above — which walks the sentences —
	// never reaches it. Named here, or the classification of a rule the mapper
	// does return has nothing holding it.
	if !(&RelationshipConflictError{Constraint: "uq_rel_works_with"}).AlreadyRecorded() {
		t.Error("uq_rel_works_with is keyed on the pair being inserted, so its refusal does mean the " +
			"edge is already on file")
	}
	// A rule this type has never seen is not evidence that anything landed.
	if (&RelationshipConflictError{Constraint: "uq_something_new"}).AlreadyRecorded() {
		t.Error("an unrecognised constraint was read as the edge already being on file")
	}
}
