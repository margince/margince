// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutcomeChecksFollowTransitionWrappersButNotUncountedCycles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := `package sample
 type result struct{}
 func (result) RowsAffected() int64 { return 1 }
 func counted() { var tag result; tag.RowsAffected() }
 func wrapped() { counted() }
 func outer() { wrapped() }
 type writer struct{}
 func NewWriter() *writer { return nil }
 func (*writer) checked() { var tag result; tag.RowsAffected() }
 func (*writer) countedMove() { var tag result; tag.RowsAffected() }
 func constructedWriter() { owner:=NewWriter(); owner.countedMove() }
 type impostor struct{}
 func (*impostor) checked() {}
 func unrelatedReceiver(owner *impostor) { owner.checked() }
 type tx struct{}
 func (tx) QueryRow(string) {}
 func (tx) Exec(string) {}
 func lockOnly() { var db tx; db.QueryRow("SELECT id FROM contact FOR UPDATE") }
 func lockWrapper() { lockOnly() }
 func foreignWrapper() { var db tx; db.QueryRow("SELECT 1") }
 func uncheckedUpdate() { var db tx; db.Exec("UPDATE contact SET full_name='Name' WHERE id=$1 AND version=$2"); lockWrapper() }
 func returning() { var db tx; db.QueryRow("UPDATE contact SET full_name='Name' WHERE id=$1 RETURNING id") }
 func unchecked() { cycle() }
 func cycle() { unchecked() }
 `
	if err := os.WriteFile(filepath.Join(dir, "writes.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	checks := countingHelpers(t, map[string]map[string]bool{}, dir)
	for _, name := range []string{"counted", "wrapped", "outer", "returning", "constructedWriter"} {
		if !checks[name] {
			t.Errorf("%s lost its delegated outcome check", name)
		}
	}
	for _, name := range []string{"unchecked", "cycle", "lockOnly", "lockWrapper", "uncheckedUpdate", "foreignWrapper", "unrelatedReceiver"} {
		if checks[name] {
			t.Errorf("%s was credited with an outcome nobody checks", name)
		}
	}
}

func TestRetirableWritesIncludeNamedAndPositionalArguments(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		"UPDATE contact SET full_name = $2 WHERE id = $1",
		"UPDATE contact SET full_name = @name WHERE id = @id",
		"DELETE FROM contact WHERE id = @record_id",
	} {
		if !byIDWriteOf.MatchString(statement) {
			t.Errorf("the retirement census missed %q", statement)
		}
	}
	if byIDWriteOf.MatchString("UPDATE contact SET full_name = @name WHERE owner_id = @owner") {
		t.Fatal("a set-based owner update was mistaken for a by-id write")
	}
}
