// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The command writes, from the committed records, the table the runtime reads.
func TestTheCommandWritesTheCommittedTable(t *testing.T) {
	out := filepath.Join(t.TempDir(), "attemptlimits_gen.go")
	if err := generate(out, filepath.Join("..", "records")); err != nil {
		t.Fatalf("generate: %v", err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join("..", "..", "..", "modules", "ai", "attemptlimits_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, committed) {
		t.Fatal("the command wrote a table other than the committed one — run `make gen`")
	}
}

// A records directory that holds an unreadable record fails the command.
func TestAnUnreadableRecordFailsTheCommand(t *testing.T) {
	records := t.TempDir()
	if err := os.MkdirAll(filepath.Join(records, "rewrite"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(records, "rewrite", "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generate(filepath.Join(t.TempDir(), "out.go"), records); err == nil {
		t.Fatal("a broken record generated a table")
	}
}
