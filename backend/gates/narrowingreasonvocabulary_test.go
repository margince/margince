// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// The narrowing reason is spelled twice: contacts.NarrowingReasons() and the
// column's CHECK. A value in Go and not in the CHECK fails the write at run
// time; a value in the CHECK and not in Go is one no widening rule was written
// for.

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

var narrowingReasonCheckRE = regexp.MustCompile(
	`contact_narrowing_reason_check CHECK .*narrowing_reason = ANY \(ARRAY\[([^\]]*)\]`)

func TestTheNarrowingReasonVocabularyIsTheCatalogsCheck(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	match := narrowingReasonCheckRE.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("no contact_narrowing_reason_check in %s — the constraint this gate reads has moved or been renamed",
			replyVerdictCatalog)
	}
	fromDB := splitCatalogTokens(string(match[1]))
	var fromGo []string
	for _, reason := range contacts.NarrowingReasons() {
		fromGo = append(fromGo, string(reason))
	}
	sort.Strings(fromGo)
	if !equalSets(fromGo, fromDB) {
		t.Errorf("contacts.NarrowingReasons() spells %v and the CHECK allows %v", fromGo, fromDB)
	}
}
