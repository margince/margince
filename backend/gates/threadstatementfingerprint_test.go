// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type threadStatementFinding struct {
	Source, Function, SQL, Fingerprint, Alias, VariantFingerprint string
	Resolved                                                      bool
	ProducerKey                                                   string
	Dependencies                                                  int
	Variants                                                      int
}

func threadStatementFingerprint(statement string) string {
	digest := sha256.Sum256([]byte(strings.Join(strings.Fields(statement), " ")))
	return hex.EncodeToString(digest[:])
}

func threadWriteFindings(t *testing.T, findings []threadStatementFinding) {
	t.Helper()
	if len(findings) == 0 {
		return
	}
	output := threadEvidenceOutput(t)
	encoded, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(output, "thread-statement-findings.json")
	if err := os.WriteFile(file, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Expanded statement evidence:", file)
}

func threadStatementSetFingerprint(statements []string) (string, bool) {
	if len(statements) == 0 {
		return "", false
	}
	unique := map[string]bool{}
	for _, statement := range statements {
		if !looksLikeSQL(stripNoise(statement)) || threadStructuralUnknown(statement) || strings.Contains(statement, threadBooleanAtom) {
			return "", false
		}
		unique[strings.Join(strings.Fields(statement), " ")] = true
	}
	ordered := make([]string, 0, len(unique))
	for statement := range unique {
		ordered = append(ordered, statement)
	}
	sort.Strings(ordered)
	digest := sha256.Sum256([]byte(strings.Join(ordered, "\n")))
	return hex.EncodeToString(digest[:]), true
}

func threadStructuralUnknown(statement string) bool {
	for index, char := range statement {
		if char == '?' && (index == 0 || statement[index-1] != '$') {
			return true
		}
	}
	return false
}

const threadBooleanAtom = "__thread_boolean_atom"

func threadEvidenceOutput(t *testing.T) string {
	t.Helper()
	if setting := flag.Lookup("test.outputdir"); setting != nil && setting.Value.String() != "" {
		return setting.Value.String()
	}
	return t.TempDir()
}
