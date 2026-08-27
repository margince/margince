// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts content at a path inside a temp dir and returns the path.
func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "margince.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return path
}

// A UTF-8 byte-order mark opens this file whenever Windows tooling wrote it:
// Notepad can leave one, and `Set-Content -Encoding UTF8` leaves one on Windows
// PowerShell 5.1 whether it was asked to or not.
//
// The mark must not decide whether the app starts. A user reading the refusal
// sees a line that is plainly a comment and has nothing to act on, because the
// three bytes in front of it do not show up in an editor.
//
// It is a property of the file, so it is stripped where the file is read rather
// than on every line.
func TestLoadEnvFileAcceptsAByteOrderMarkBeforeAComment(t *testing.T) {
	path := write(t, "\ufeff# Margince settings.\nMARGINCE_PORT=8800\n")

	env, err := loadEnvFile(path)
	if err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if len(env) != 1 || env[0] != "MARGINCE_PORT=8800" {
		t.Errorf("env = %q, want [MARGINCE_PORT=8800]", env)
	}
}

// The same mark in front of a line that IS an assignment is the worse half of
// the same bug, because nothing refuses it: `strings.Cut` finds the "=", and the
// child process is handed a variable whose name begins with an invisible
// character. The setting is silently ignored, which is the outcome parseEnvLine's
// own comment says an error exists to prevent.
func TestLoadEnvFileReadsASettingBehindAByteOrderMark(t *testing.T) {
	path := write(t, "\ufeffMARGINCE_PORT=8800\n")

	env, err := loadEnvFile(path)
	if err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if len(env) != 1 || env[0] != "MARGINCE_PORT=8800" {
		t.Errorf("env = %q, want [MARGINCE_PORT=8800] with no mark in the name", env)
	}
}

// Stripping happens for the FIRST line only, so an invisible character anywhere
// else still has to be refused rather than carried into a variable name. A
// zero-width or non-breaking character in a key cannot be seen in an editor, and
// a user who cannot see it cannot debug the setting that never took effect.
func TestParseEnvLineRefusesAnInvisibleCharacterInAKey(t *testing.T) {
	for _, raw := range []string{
		"\ufeffMARGINCE_PORT=8800", // a mark on a line that is not line 1
		"MARGINCE\u00a0PORT=8800",  // a non-breaking space
		"MARGINCE_PORT\u200b=8800", // a zero-width space
	} {
		if _, err := parseEnvLine(raw); err == nil {
			t.Errorf("parseEnvLine(%q) = no error, want one naming the character", raw)
		}
	}
}

// The template the launcher writes on a first start must be readable by the
// parser that reads it back. It is the one env file every installation has, and
// nothing else proves the two halves agree.
func TestEnvTemplateLoads(t *testing.T) {
	path := write(t, envTemplate)

	env, err := loadEnvFile(path)
	if err != nil {
		t.Fatalf("the shipped template does not load: %v", err)
	}
	// Everything in it is commented out, so a fresh install runs the defaults.
	if len(env) != 0 {
		t.Errorf("the template sets %q; every line in it should be commented out", env)
	}
}

// The rules the file already had, held down because it carried no tests: a
// change that fixes the mark must not quietly cost `export`, the quote
// stripping, or the refusal of a line with no "=".
func TestParseEnvLineKeepsItsExistingRules(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"a blank line", "   ", ""},
		{"a comment", "  # a note", ""},
		{"a plain assignment", "MARGINCE_PORT=8800", "MARGINCE_PORT=8800"},
		{"an export a user pasted", "export MARGINCE_PORT=8800", "MARGINCE_PORT=8800"},
		{"a double-quoted value", `NAME="two words"`, "NAME=two words"},
		{"a single-quoted value", `NAME='two words'`, "NAME=two words"},
		{"an empty value", "MARGINCE_LICENSE=", "MARGINCE_LICENSE="},
		{"a value holding an =", "DSN=k=v", "DSN=k=v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnvLine(tc.raw)
			if err != nil {
				t.Fatalf("parseEnvLine(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("parseEnvLine(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}

	for _, tc := range []struct{ name, raw string }{
		{"no equals sign", "MARGINCE_PORT 8800"},
		{"no name before the equals", "=8800"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseEnvLine(tc.raw); err == nil {
				t.Errorf("parseEnvLine(%q) = no error, want one", tc.raw)
			}
		})
	}
}

// The error names the file and the line, because it is read by someone who has
// a text editor open and no stack trace.
func TestLoadEnvFileNamesTheFileAndLine(t *testing.T) {
	path := write(t, "# fine\n\nMARGINCE_PORT 8800\n")

	_, err := loadEnvFile(path)
	if err == nil {
		t.Fatal("loadEnvFile accepted a line with no '='")
	}
	if !strings.Contains(err.Error(), "line 3") || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want it to name %s and line 3", err, path)
	}
}
