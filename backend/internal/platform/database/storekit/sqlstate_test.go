// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A number bound into an int4 column fails inside pgx, before any statement is
// sent; the error built here is the one pgx itself raises on that path.
func TestIsInvalidValueForType_pgxOverflowWording(t *testing.T) {
	sd := &pgconn.StatementDescription{ParamOIDs: []uint32{pgtype.Int4OID}}
	for name, arg := range map[string]int{"above": 3_000_000_000, "below": -3_000_000_000} {
		t.Run(name, func(t *testing.T) {
			var eqb pgx.ExtendedQueryBuilder
			err := eqb.Build(pgtype.NewMap(), sd, []any{arg})
			if err == nil {
				t.Fatalf("pgx bound %d into an int4 without error", arg)
			}
			if !IsInvalidValueForType(fmt.Errorf("insert lead source: %w", err)) {
				t.Fatalf("pgx's refusal %q is not read as a value the caller must change", err)
			}
		})
	}
}

func TestIsInvalidValueForType_sqlstates(t *testing.T) {
	cases := map[string]bool{
		"22021": true, // NUL or malformed UTF-8
		"22P05": true, // NUL inside a jsonb value
		"22P02": true,
		"22003": true,
		"22012": false, // division by zero is the server's sum
		"23505": false,
	}
	for code, want := range cases {
		err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code})
		if got := IsInvalidValueForType(err); got != want {
			t.Errorf("SQLSTATE %s: got %v, want %v", code, got, want)
		}
	}
}

func TestIsInvalidValueForType_ignoresUnrelatedErrors(t *testing.T) {
	for _, err := range []error{nil, errors.New("connection refused"), errors.New("failed to encode args[0]: unsupported type")} {
		if IsInvalidValueForType(err) {
			t.Errorf("%v read as a caller's bad value", err)
		}
	}
}

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
	if got := NameTaken(label, "a role", "Owner", "role_label_once").Error(); got != `conflict: a role called "Owner" already exists` {
		t.Errorf("the conflict reads %q", got)
	}
	if NameTaken(nil, "a role", "Owner", "role_label_once") != nil {
		t.Error("no error answered an error")
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
