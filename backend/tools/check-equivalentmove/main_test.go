// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSQL(t *testing.T, dir, file, sql string) string {
	t.Helper()
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func moveArgs(oldVersion, newVersion, oldUp, oldDown, newUp, newDown string) []string {
	return []string{
		"-namespace", "core", "-name", "probe", "-old", oldVersion, "-new", newVersion,
		"-old-up", oldUp, "-old-down", oldDown, "-new-up", newUp, "-new-down", newDown,
	}
}

func TestOnlyTheChecksTheMigratorRunsAdmitAMove(t *testing.T) {
	dir := t.TempDir()
	up := writeSQL(t, dir, "up.sql", "SELECT 1;\n")
	down := writeSQL(t, dir, "down.sql", "SELECT 2;\n")
	edited := writeSQL(t, dir, "edited.sql", "SELECT 3;\n")

	cases := map[string]struct {
		args []string
		want int
	}{
		"the same content at the same version": {moveArgs("1700000001", "1700000001", up, down, up, down), admitted},
		"edited content with no checked equivalence": {
			moveArgs("1700000001", "1700000002", up, down, edited, down), notAdmitted,
		},
		"a missing SQL half":  {moveArgs("1700000001", "1700000002", up, "", up, down), cannotTell},
		"an unreadable file":  {moveArgs("1700000001", "1700000002", up, filepath.Join(dir, "absent.sql"), up, down), cannotTell},
		"an unknown argument": {[]string{"-surprise"}, cannotTell},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got, verdict := run(tc.args); got != tc.want {
				t.Fatalf("exit %d, want %d (%s)", got, tc.want, verdict)
			}
		})
	}
}
