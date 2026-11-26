// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestRereadSecretReadsAReplacedSecretFile(t *testing.T) {
	cfg, err := load(t, fixture{
		env:   []string{"MARGINCE_DSN=${file:{dir}/dsn}"},
		files: map[string]string{"dsn": "postgres://old\n"},
	}, dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, origin := config.ValWithSource(cfg, dsn)
	if err := os.WriteFile(origin.File, []byte("postgres://new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, err := config.RereadSecret(cfg, dsn)
	if err != nil || got != "postgres://new" {
		t.Fatalf("RereadSecret = %q, %v; want the replaced file's value", got, err)
	}
	// Load's own answer is unchanged: only RereadSecret reads the file again.
	if v := config.Val(cfg, dsn); v != "postgres://old" {
		t.Errorf("Val = %q after the re-read, want the value Load resolved", v)
	}
}

// The re-read runs the same checks Load runs, so a file emptied on disk is a
// fault rather than an empty secret.
func TestRereadSecretRefusesAnEmptiedSecretFile(t *testing.T) {
	cfg, err := load(t, fixture{
		env:   []string{"MARGINCE_DSN=${file:{dir}/dsn}"},
		files: map[string]string{"dsn": "postgres://old\n"},
	}, dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, origin := config.ValWithSource(cfg, dsn)
	if err := os.WriteFile(origin.File, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := config.RereadSecret(cfg, dsn); err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("RereadSecret on an emptied file: %v, want the empty-value refusal", err)
	}
}

// A secret that is not a ${file:} reference is fixed at start, so the re-read
// answers what Load resolved.
func TestRereadSecretAnswersAnEnvironmentSecretAsLoaded(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"MARGINCE_DSN=${env:OTHER}", "OTHER=postgres://env"}}, dsn)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := config.RereadSecret(cfg, dsn)
	if err != nil || got != "postgres://env" {
		t.Fatalf("RereadSecret = %q, %v; want the loaded value", got, err)
	}
}
