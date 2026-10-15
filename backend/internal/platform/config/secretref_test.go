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

var relayPassword = config.Define(config.Spec[string]{
	Key: "smtp.password", Env: "MARGINCE_SMTP_PASSWORD", Type: config.String, Secret: true, AllowNone: true, Doc: "d",
})

func TestNoneDeclaresASecretAbsentWhereTheSettingAcceptsIt(t *testing.T) {
	for name, f := range map[string]fixture{
		"yaml": {files: map[string]string{"margince.yaml": "smtp:\n  password: ${none}\n"}},
		"env":  {env: []string{"MARGINCE_SMTP_PASSWORD=${none}"}},
	} {
		cfg, err := load(t, f, relayPassword)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		v, origin := config.ValWithSource(cfg, relayPassword)
		if v != "" || !origin.None || origin.Source == config.SourceUnset {
			t.Fatalf("%s: got %q %+v; want an empty value marked None", name, v, origin)
		}
	}
	cfg, err := load(t, fixture{}, relayPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, origin := config.ValWithSource(cfg, relayPassword); origin.None {
		t.Fatal("a secret nobody declared is not one declared absent")
	}
}

func TestNoneIsRefusedWhereTheSettingDoesNotAcceptIt(t *testing.T) {
	_, err := load(t, fixture{files: map[string]string{"margince.yaml": "dsn: ${none}\n"}}, dsn)
	if err == nil || !strings.Contains(err.Error(), "does not accept") {
		t.Fatalf("got %v", err)
	}
}

func TestAReferenceThatHoldsNoneIsRefused(t *testing.T) {
	_, err := load(t, fixture{
		files: map[string]string{"margince.yaml": "smtp:\n  password: ${env:RELAY}\n"},
		env:   []string{"RELAY=${none}"},
	}, relayPassword)
	if err == nil || !strings.Contains(err.Error(), "resolves to ${none}") {
		t.Fatalf("got %v", err)
	}
}

func TestASecretFileTooLargeToBeASecretIsRefused(t *testing.T) {
	_, err := load(t, fixture{
		env:   []string{"MARGINCE_DSN=${file:{dir}/dsn}"},
		files: map[string]string{"dsn": strings.Repeat("x", 64<<10+1)},
	}, dsn)
	if err == nil || !strings.Contains(err.Error(), "more than a secret holds") || strings.Contains(err.Error(), "xxxx") {
		t.Fatalf("got %v", err)
	}
}
