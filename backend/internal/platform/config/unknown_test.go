// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// ns is the namespace, spelled apart from every suffix below for the reason
// unknown.go gives: a quoted MARGINCE_* literal is read by the documentation
// gate as a variable somebody must document, and these are fixtures.
const ns = "MARGINCE_"

// warningsFor loads with environ and returns the report's names.
func warningsFor(t *testing.T, environ ...string) []string {
	t.Helper()
	cfg, err := load(t, fixture{env: environ}, level)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Warnings()
}

func TestTheReportNeverCarriesAValue(t *testing.T) {
	// A misspelled secret is still a secret. The report names variables, so a
	// typo'd credential is reported without publishing what it holds.
	const credential = "a-real-bearer-token"
	got := warningsFor(t, ns+"TOKKEN="+credential)
	for _, name := range got {
		if strings.Contains(name, credential) {
			t.Fatalf("the report carried the value: %q", name)
		}
	}
	if len(got) != 1 || got[0] != ns+"TOKKEN" {
		t.Fatalf("warnings = %v, want exactly the misspelling", got)
	}
}

// A variable a ${env:NAME} reference reads is consumed, wherever the reference
// sits. Reporting it would tell the operator to delete what the boot depends on.
func TestAVariableAReferenceReadsIsNotReported(t *testing.T) {
	reg, src := sources(t, fixture{
		args: []string{"--env-file", "{dir}/margince.env"},
		env: []string{
			ns + "MAIL_PW=from-yaml-bootstrap", ns + "SIBLING_PW=from-yaml-sibling",
			ns + "DB_PW=from-env-file", ns + "STRAY=nobody-reads-me",
		},
		files: map[string]string{
			"margince.yaml": "workspace:\n  smtp:\n    password: ${env:" + ns + "MAIL_PW}\n" +
				"worker_token: ${env:" + ns + "SIBLING_PW}\n",
			"margince.env": ns + "DSN=${env:" + ns + "DB_PW}\n",
		},
	}, level, dsn)
	src.BootstrapRoots = map[string]bool{"workspace": true}
	src.Known = config.Names{Keys: []string{"worker_token"}}
	cfg, err := config.Load(reg, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Warnings(); len(got) != 1 || got[0] != ns+"STRAY" {
		t.Fatalf("warnings = %v, want only the variable nothing references", got)
	}
}

func TestTheReportIgnoresWhatIsNotOursToJudge(t *testing.T) {
	got := warningsFor(t,
		"PATH=/usr/bin",                // the platform's, not ours
		ns+"TEST_DSN=postgres://x",     // the suite's own plumbing
		ns+"BENCH_RECORD=1",            // likewise
		ns+"ADMIN_PASSWORD=x",          // the container entrypoint's
		ns+"BUILD_REVISION=abc123",     // stamped into the image
		ns+"COMPOSITION_FRONTEND=stub", // likewise
		ns+"ENV=dev",                   // the posture, which Load itself reads
		// Undeclared and empty isolates the rule: a declared name would be
		// excluded before the empty value mattered.
		ns+"NOT_A_DECLARED_NAME=",
	)
	if len(got) != 0 {
		t.Errorf("warnings = %v; a report that cries wolf gets ignored, which costs it the typo it exists to catch", got)
	}
}

func TestAnEntryWithNoValueIsNotAVariable(t *testing.T) {
	// os.Environ yields NAME=value; anything without the separator is not
	// something an operator set, and reading it as a name would invent one.
	if got := warningsFor(t, ns+"NO_SEPARATOR"); len(got) != 0 {
		t.Errorf("warnings = %v, want none", got)
	}
}

func TestTheReportIsOrdered(t *testing.T) {
	// A map's iteration order would make the same environment produce a
	// different line every boot, and a diff of two boots would read as change.
	got := warningsFor(t, ns+"ZEBRA=1", ns+"ALPHA=1")
	if len(got) != 2 || got[0] != ns+"ALPHA" || got[1] != ns+"ZEBRA" {
		t.Fatalf("warnings = %v, want alphabetical", got)
	}
}

// The warning names the variables without their values. It must not claim
// more than the registries can know.
func TestTheWarningNamesVariablesWithoutValues(t *testing.T) {
	const credential = "a-real-bearer-token"
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	config.WarnUndeclared(logger, warningsFor(t, ns+"TOKKEN="+credential, ns+"REDDIS=localhost:6379"))
	line := buf.String()

	if strings.Contains(line, credential) {
		t.Errorf("the warning carried a credential: %s", line)
	}
	for _, name := range []string{ns + "TOKKEN", ns + "REDDIS"} {
		if !strings.Contains(line, name) {
			t.Errorf("the warning does not name %s: %s", name, line)
		}
	}
	// An operator who reads "no such variable" about a name some other process
	// reads, such as the entrypoint, will delete working configuration.
	if strings.Contains(line, "no such configuration variable") {
		t.Errorf("the warning claims a variable does not exist, which one role cannot know: %s", line)
	}
}

func TestNothingIsLoggedWhenThereIsNothingToSay(t *testing.T) {
	// A line on every boot of every correctly-configured installation is a line
	// operators learn to scroll past, and it would take the real one with it.
	var buf bytes.Buffer
	config.WarnUndeclared(slog.New(slog.NewTextHandler(&buf, nil)), nil)
	if buf.Len() != 0 {
		t.Errorf("logged %q with nothing to report", buf.String())
	}
}
