// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every sender kind is JUDGED by the reconnect lane, one way or the other.
//
// PrivateSenderClause names the kinds that are not a business relationship. A
// kind added to the vocabulary and not named there is silently treated as one —
// it would reach a lane promising new revenue with nobody having decided it
// should. The failure is invisible in production: the row simply appears, and
// looks like every other contact.
//
// The vocabulary is READ FROM THE SCHEMA rather than listed here, because a
// list here is a second copy of the thing under test: enrolling a kind would
// leave this green while the new kind walked straight through the predicate.
// The CHECK constraint is what the database actually enforces, so it is the
// honest source.
func TestEveryVerdictKindIsJudgedByTheReconnectLane(t *testing.T) {
	all := senderKindsFromSchema(t)
	if len(all) < 8 {
		t.Fatalf("read %d kinds out of the schema, want the full vocabulary — a "+
			"census that reads a shorter list passes over what it cannot see", len(all))
	}

	// The kinds a reconnect lane may show: a real person, and the two shapes of
	// company that correspond under their own name. Everything else is
	// private life or noise.
	business := map[string]bool{
		KindPerson:        true,
		KindRoleMailbox:   true,
		KindCompanySender: true,
	}
	clause := PrivateSenderClause("e", "$1")
	for _, kind := range all {
		named := strings.Contains(clause, "'"+kind+"'")
		if business[kind] && named {
			t.Errorf("%q is a business counterparty but the reconnect lane excludes it, "+
				"so a real lapsed relationship never reaches the reader", kind)
		}
		if !business[kind] && !named {
			t.Errorf("%q is not a business relationship and the reconnect lane does not "+
				"exclude it — it will appear under a heading promising new revenue "+
				"with nobody having decided it should", kind)
		}
	}
}

// senderKindsFromSchema reads the sender vocabulary out of the LAST migration
// that constrains it, which is what the column actually permits.
func senderKindsFromSchema(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "migrations", "core")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	// Migrations are named for the second they were written, so the last one
	// naming this constraint is the one in force.
	sortStrings(names)
	pattern := regexp.MustCompile(
		`capture_pending_counterparty_kind_check[\s\S]*?kind IN \(([^)]*)\)`)
	var kinds []string
	for _, name := range names {
		body, err := os.ReadFile(filepath.Clean(filepath.Join(dir, name)))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		found := pattern.FindStringSubmatch(string(body))
		if found == nil {
			continue
		}
		kinds = nil
		for _, raw := range strings.Split(found[1], ",") {
			if trimmed := strings.Trim(strings.TrimSpace(raw), "'"); trimmed != "" {
				kinds = append(kinds, trimmed)
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatal("no migration constrains the sender kinds, so this census read nothing")
	}
	return kinds
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}
