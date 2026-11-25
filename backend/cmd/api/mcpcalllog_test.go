// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func callLogConfig(t *testing.T, env ...string) apiConfig {
	t.Helper()
	cfg, _, err := loadAPIConfig([]string{"--dsn", "postgres://user@localhost:5432/db"}, append(env, "MARGINCE_ENV=dev"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func TestAnUnsetCallLogOpensNoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calls.jsonl")
	for name, cfg := range map[string]apiConfig{
		"unset":         callLogConfig(t),
		"connector off": callLogConfig(t, "MARGINCE_MCP_CALL_LOG="+path),
	} {
		cfg.mcpConnector = name == "unset"
		opts, closeLog, err := mcpCallLogLane(cfg, slog.New(slog.DiscardHandler))
		if err != nil || len(opts) != 0 {
			t.Errorf("%s: options %d, err %v; want none", name, len(opts), err)
		}
		closeLog()
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("an off call log left %d file(s) (err %v), want none", len(entries), err)
	}
}

func TestASetCallLogOpensItsFileAndSaysSoAtBoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls.jsonl")
	cfg := callLogConfig(t, "MARGINCE_MCP_CALL_LOG="+path)
	cfg.mcpConnector = true
	var logged bytes.Buffer
	opts, closeLog, err := mcpCallLogLane(cfg, slog.New(slog.NewTextHandler(&logged, nil)))
	if err != nil || len(opts) != 1 {
		t.Fatalf("options %d, err %v; want the call log", len(opts), err)
	}
	closeLog()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the call log was not opened: %v", err)
	}
	if !strings.Contains(logged.String(), "personal data") || !strings.Contains(logged.String(), path) {
		t.Errorf("boot logged %q, want the file named and its personal data said", logged.String())
	}
}

func TestACallLogInAFolderThatIsNotThereStopsTheBoot(t *testing.T) {
	cfg := callLogConfig(t, "MARGINCE_MCP_CALL_LOG="+filepath.Join(t.TempDir(), "missing", "calls.jsonl"))
	cfg.mcpConnector = true
	if _, _, err := mcpCallLogLane(cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("a call log that cannot be opened booted anyway")
	}
}

// Its lines hold personal data that no retention policy or erasure reaches, so
// a production install refuses to boot with it.
func TestACallLogIsRefusedInProduction(t *testing.T) {
	path := "MARGINCE_MCP_CALL_LOG=" + filepath.Join(t.TempDir(), "calls.jsonl")
	dsn := []string{"--dsn", "postgres://user@localhost:5432/db"}
	for _, posture := range []string{"production", ""} {
		_, _, err := loadAPIConfig(dsn, []string{path, "MARGINCE_ENV=" + posture})
		if err == nil || !strings.Contains(err.Error(), "mcp.call_log") || !strings.Contains(err.Error(), "production") {
			t.Errorf("MARGINCE_ENV=%q: err = %v, want a boot fault naming mcp.call_log and production", posture, err)
		}
	}
	if _, _, err := loadAPIConfig(dsn, []string{path, "MARGINCE_ENV=test"}); err != nil {
		t.Errorf("a test install with a call log: %v, want it allowed", err)
	}
	if _, _, err := loadAPIConfig(dsn, []string{"MARGINCE_ENV=production"}); err != nil {
		t.Errorf("a production install with no call log: %v, want it allowed", err)
	}
}
