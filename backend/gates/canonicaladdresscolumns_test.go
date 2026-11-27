// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A column that holds an address or a domain and takes part in a unique index
// is stored lowercased and trimmed. The database holds that with a CHECK.
//
// A unique index over the raw value sees "A@x.com" and "a@x.com" as two rows,
// while a reader that folds sees one. The CHECK makes the stored form the
// folded one, so the index and every reader agree.
//
// The corpus derives from the head catalog: every unique index, and each
// column in it whose name says address, email or domain.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	// uniqueIndexLine reads a unique index: its table and what it indexes.
	// A partial index's predicate is cut off first (indexedPart), so a column
	// named only there does not count as indexed.
	uniqueIndexLine = regexp.MustCompile(`^public\.\w+ CREATE UNIQUE INDEX \w+ ON public\.(\w+) USING \w+ \((.*)\)(?: NULLS NOT DISTINCT)?$`)
	// textColumnLine reads a text column: its table and its name.
	textColumnLine = regexp.MustCompile(`^public\.(\w+)\.(\w+) text\b`)
	// checkLine reads a CHECK constraint: its table and its definition.
	checkLine = regexp.MustCompile(`^public\.(\w+)\.\w+ CHECK (.*)$`)
	// addressName is a column name that says the value is an address or a domain.
	addressName = regexp.MustCompile(`email|address|domain`)
)

// notAnAddress records each such column that needs no canonical-form CHECK,
// with why. An unlisted column must carry the CHECK.
var notAnAddress = gatekit.Waive(map[string]string{
	"communication_decision.recipient_address": "a recipient key of two shapes: a folded address, or a channel id whose case its provider may tell apart. decisionRecipientKey folds the address shape at the write",
})

// indexedPart is an index line without the predicate of a partial index.
func indexedPart(line string) string {
	before, _, _ := strings.Cut(line, " WHERE (")
	return before
}

// holdsCanonicalForm reports whether a CHECK definition pins column to its
// lowercased form. The trimmed spelling and the older lowercase-only one count.
func holdsCanonicalForm(def, column string) bool {
	for _, form := range []string{
		"(" + column + " = lower(btrim(" + column + ")))",
		"(" + column + " = lower(" + column + "))",
	} {
		if strings.Contains(def, form) {
			return true
		}
	}
	return false
}

// holdsOneSpelling reports whether a CHECK definition holds column to the
// spelling values writes: composed accents and nothing that draws nothing.
func holdsOneSpelling(def, column string) bool {
	return strings.Contains(def, "(is_identifier_text("+column+"))")
}

// addressColumnsInUniqueIndexes derives the corpus, mapping table.column to
// whether CHECKs hold both its canonical form and its one spelling.
func addressColumnsInUniqueIndexes(t *testing.T) map[string]bool {
	t.Helper()
	raw := headCatalog(t)
	columns := map[string][]string{}
	checks := map[string][]string{}
	var indexes [][2]string
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if m := textColumnLine.FindStringSubmatch(line); m != nil {
			columns[m[1]] = append(columns[m[1]], m[2])
		}
		if m := checkLine.FindStringSubmatch(line); m != nil {
			checks[m[1]] = append(checks[m[1]], m[2])
		}
		if m := uniqueIndexLine.FindStringSubmatch(indexedPart(line)); m != nil {
			indexes = append(indexes, [2]string{m[1], m[2]})
		}
	}
	held := map[string]bool{}
	for _, index := range indexes {
		table, indexed := index[0], index[1]
		for _, column := range columns[table] {
			if !addressName.MatchString(column) || !regexp.MustCompile(`\b`+regexp.QuoteMeta(column)+`\b`).MatchString(indexed) {
				continue
			}
			key := table + "." + column
			lowered, spelled := false, false
			for _, def := range checks[table] {
				lowered = lowered || holdsCanonicalForm(def, column)
				spelled = spelled || holdsOneSpelling(def, column)
			}
			held[key] = lowered && spelled
		}
	}
	return held
}

func TestEveryUniqueAddressColumnHoldsItsCanonicalForm(t *testing.T) {
	t.Parallel()
	defer notAnAddress.AssertAllMatched(t)
	held := addressColumnsInUniqueIndexes(t)
	if len(held) < 8 {
		t.Fatalf("the catalog yields %d address columns in unique indexes; the reader has stopped "+
			"matching the catalog's line shape", len(held))
	}
	keys := make([]string, 0, len(held))
	for key := range held {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if held[key] || notAnAddress.Waived(t, key) {
			continue
		}
		t.Errorf("%s takes part in a unique index and no CHECK holds it lowercased and trimmed, or in "+
			"one spelling, so two spellings of one address are two rows. Add CHECK (col = lower(btrim(col))) "+
			"and CHECK (is_identifier_text(col)) in a migration, NOT VALID first, or waive the column in "+
			"notAnAddress with what it holds.", key)
	}
}

// The reader is held to the shapes it must see, and to the forms it accepts.
func TestTheCanonicalFormReaderSeesEachShape(t *testing.T) {
	t.Parallel()
	if m := uniqueIndexLine.FindStringSubmatch("public.uq_app_user_email CREATE UNIQUE INDEX uq_app_user_email ON public.app_user USING btree (lower(email))"); m == nil || m[1] != "app_user" || m[2] != "lower(email)" {
		t.Errorf("the reader read an expression index as %v", m)
	}
	if m := uniqueIndexLine.FindStringSubmatch(indexedPart("public.x CREATE UNIQUE INDEX x ON public.t USING btree (email) WHERE (status = 'live'::text)")); m == nil || m[2] != "email" {
		t.Errorf("the reader read a partial index as %v", m)
	}
	if m := uniqueIndexLine.FindStringSubmatch("public.x CREATE UNIQUE INDEX x ON public.t USING btree (seat, email) NULLS NOT DISTINCT"); m == nil || m[2] != "seat, email" {
		t.Errorf("the reader read a key whose nulls are equal as %v", m)
	}
	if uniqueIndexLine.MatchString("public.x CREATE INDEX x ON public.t USING btree (email)") {
		t.Error("the reader took a plain index for a unique one")
	}
	for def, want := range map[string]bool{
		"((email = lower(btrim(email))))":                       true,
		"((email = lower(email)))":                              true,
		"(((domain = lower(domain)) AND (domain <> ''::text)))": true,
		"((email <> ''::text))":                                 false,
		"((other_email = lower(btrim(other_email))))":           false,
	} {
		if got := holdsCanonicalForm(def, "email") || holdsCanonicalForm(def, "domain"); got != want {
			t.Errorf("holdsCanonicalForm(%q) = %v, want %v", def, got, want)
		}
	}
	if !holdsOneSpelling("(is_identifier_text(email))", "email") || holdsOneSpelling("(is_identifier_text(other_email))", "email") {
		t.Error("holdsOneSpelling misread which column the spelling check holds")
	}
}
