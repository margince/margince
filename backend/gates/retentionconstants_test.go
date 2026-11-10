// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestRetentionReadsSharedConstantsAndRefusesUnknownAssignments(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, assignment, fragment                string
		unreadable, bodyErased, retainedDestroyed bool
	}{
		{"shared erasure", "body = NULL", "wipe", false, true, false},
		{"shared destruction", "counterparty_email = NULL", "wipe", false, false, true},
		{"runtime assignment", "body = NULL", "extra", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sweep.go")
			source := "package sample\nconst wipe = " + strconv.Quote(tc.assignment) +
				"\nfunc f(extra string) string { return " + strconv.Quote("UPDATE activity SET ") +
				" + " + tc.fragment + " + " + strconv.Quote(" WHERE id = $1") + " }\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			consts := sqlConstants(t, dir)
			var statements []string
			for _, literal := range sqlLiterals(t, path, consts) {
				for _, statement := range sqlStatements(literal) {
					if slices.Contains(sqlWriteTargets(statement), "activity") {
						statements = append(statements, statement)
					}
				}
			}
			assembled := assembledSweepTargets(t, path, consts)
			if got := unreadableWriteOn(statements, assembled["activity"]) != ""; got != tc.unreadable {
				t.Fatalf("unreadable=%v, want %v", got, tc.unreadable)
			}
			if got := oneStatementCarries(statements, []string{"body = NULL"}); got != tc.bodyErased {
				t.Errorf("body erased=%v, want %v", got, tc.bodyErased)
			}
			if got := statementDestroying(statements, "activity", "counterparty_email") != ""; got != tc.retainedDestroyed {
				t.Errorf("retained column destroyed=%v, want %v", got, tc.retainedDestroyed)
			}
		})
	}
}
