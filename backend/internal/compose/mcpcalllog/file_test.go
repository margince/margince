// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpcalllog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

func TestEachCallIsOneLineWithItsSecretsStripped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls.jsonl")
	file, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	file.Record(ctx, agents.CallRecord{Tool: "read_record", Outcome: agents.OutcomeOK, Answer: "found it"})
	file.Record(ctx, agents.CallRecord{
		Tool: "set_up_partner", Outcome: agents.OutcomeToolError,
		Arguments: json.RawMessage(`{"api_key":"AKIAIOSFODNN7EXAMPLE"}`),
	})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the log is mode %o, want 0600: its lines hold personal data", mode)
	}
	body, err := os.ReadFile(path) // #nosec G304 -- t.TempDir() + a literal filename, test-only
	if err != nil {
		t.Fatal(err)
	}
	var tools []string
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		var record agents.CallRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("line %q is not one JSON record: %v", scanner.Text(), err)
		}
		tools = append(tools, record.Tool)
	}
	if strings.Join(tools, ",") != "read_record,set_up_partner" {
		t.Errorf("the log holds %v, want both calls in order", tools)
	}
	if strings.Contains(string(body), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("a credential in an argument reached the log: %s", body)
	}
}

func TestAFileThatCannotBeOpenedIsAnErrorNamingTheSetting(t *testing.T) {
	if _, err := Open(t.TempDir(), slog.New(slog.DiscardHandler)); err == nil ||
		!strings.Contains(err.Error(), "mcp.call_log") {
		t.Errorf("opening a folder as the call log = %v, want an error naming the setting", err)
	}
}

func TestARecordThatCannotBeWrittenIsDroppedAndLogged(t *testing.T) {
	var logged bytes.Buffer
	file, err := Open(filepath.Join(t.TempDir(), "calls.jsonl"), slog.New(slog.NewTextHandler(&logged, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	file.Record(context.Background(), agents.CallRecord{Tool: "read_record"})
	if !strings.Contains(logged.String(), "could not be written") {
		t.Errorf("a record written after the file closed logged %q, want the drop named", logged.String())
	}
}

func TestAnExistingWiderFileIsNarrowedToItsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Set after writing, so the process umask cannot hand the test a narrow file.
	if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- the wider mode is what the test narrows
		t.Fatal(err)
	}
	file, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("an existing 0644 log is mode %o after Open, want 0600", mode)
	}
}
