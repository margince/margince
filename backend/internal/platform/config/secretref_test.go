// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestASecretReadsItsFileAndReportsWhereFrom(t *testing.T) {
	cfg, err := load(t, fixture{
		env:   []string{"MARGINCE_DSN=${file:{dir}/dsn}"},
		files: map[string]string{"dsn": "postgres://x\n"},
	}, dsn)
	if err != nil {
		t.Fatal(err)
	}
	v, origin := config.ValWithSource(cfg, dsn)
	if v != "postgres://x" || origin.Source != config.SourceEnv || !strings.HasSuffix(origin.File, "/dsn") {
		t.Fatalf("got %q %+v", v, origin)
	}
}

func TestOnlyOneTrailingNewlineIsRemoved(t *testing.T) {
	cfg, err := load(t, fixture{
		env:   []string{"MARGINCE_DSN=${file:{dir}/dsn}"},
		files: map[string]string{"dsn": "x\r\n"},
	}, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if v := config.Val(cfg, dsn); v != "x\r" {
		t.Fatalf("got %q", v)
	}
}

func TestAMissingSecretFileStopsTheBootAndNamesThePath(t *testing.T) {
	_, err := load(t, fixture{env: []string{"MARGINCE_DSN=${file:/nonexistent/dsn}"}}, dsn)
	if err == nil || !strings.Contains(err.Error(), "/nonexistent/dsn") {
		t.Fatalf("got %v", err)
	}
}

func TestAnEmptySecretFileFailsNonEmpty(t *testing.T) {
	_, err := load(t, fixture{env: []string{"MARGINCE_DSN=${file:{dir}/dsn}"}, files: map[string]string{"dsn": ""}}, dsn)
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("got %v", err)
	}
}

func TestYAMLRefusesALiteralSecret(t *testing.T) {
	_, err := load(t, fixture{files: map[string]string{"margince.yaml": "dsn: postgres://x\n"}}, dsn)
	if err == nil || !strings.Contains(err.Error(), "a secret in YAML must be a ${file:...} or ${env:...} reference") {
		t.Fatalf("got %v", err)
	}
}
