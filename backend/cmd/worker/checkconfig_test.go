// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The worker's --check-config reads the bootstrap roots too, since its boot
// reads them for the seeded model binding.
func TestTheWorkerCheckReportsBootstrapFaults(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nworkspce:\n  name: Acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	checked, err := checkWorkerConfig([]string{"--check-config", "--config", path, "--dsn", "postgres://localhost/margince_test"}, nil, &out)
	if !checked || err == nil || !strings.Contains(err.Error(), path+": unknown key workspce (line 2); did you mean workspace?") {
		t.Fatalf("checked %v, err %v; want the misspelt root once, with the name it meant", checked, err)
	}
	if !strings.Contains(out.String(), "config: read base_file "+path) {
		t.Errorf("the check printed no trace:\n%s", out.String())
	}
}
