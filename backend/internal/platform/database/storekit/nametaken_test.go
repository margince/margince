// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestNameTakenAnswersOnlyTheNamedIndexes(t *testing.T) {
	label := fmt.Errorf("insert: %w", &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "role_label_once"})
	cases := []struct {
		name     string
		err      error
		indexes  []string
		conflict bool
	}{
		{"the named index", label, []string{"role_key_key", "role_label_once"}, true},
		{"another index", label, []string{"role_key_key"}, false},
		{"another SQLSTATE", &pgconn.PgError{Code: pgForeignKeyViolation, ConstraintName: "role_label_once"}, []string{"role_label_once"}, false},
	}
	for _, c := range cases {
		got := NameTaken(c.err, "a role", "Owner", c.indexes...)
		if errors.Is(got, apperrors.ErrConflict) != c.conflict {
			t.Errorf("%s: got %v, want a conflict: %v", c.name, got, c.conflict)
		}
		if !c.conflict && got != c.err {
			t.Errorf("%s: got %v, want the error passed through unchanged", c.name, got)
		}
	}
	if NameTaken(nil, "a role", "Owner", "role_label_once") != nil {
		t.Error("no error answered an error")
	}
}

func TestANameClashAnswersNameTakenNamingTheEntry(t *testing.T) {
	clash := NameTaken(&pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "role_label_once"}, "a role", "Owner", "role_label_once")
	fault, ok := errors.AsType[apperrors.MessageFault](clash)
	if !ok {
		t.Fatalf("%v carries no code of its own", clash)
	}
	code, message := fault.MessageFault()
	if code != "name_taken" || message != `a role called "Owner" already exists; choose a different name` {
		t.Errorf("the clash reads %s %q", code, message)
	}
}

func TestNameAfterIsTheTrimmedSentNameOrTheHeldOne(t *testing.T) {
	sent := "  Renewal lead "
	if got := NameAfter("Owner", &sent); got != "Renewal lead" {
		t.Errorf("a sent name reads %q, want it trimmed", got)
	}
	if got := NameAfter("Owner", nil); got != "Owner" {
		t.Errorf("no sent name reads %q, want the held one", got)
	}
}
