// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// The relationship mark is spelled twice: relationshipmark.Values() and the
// column's CHECK. A value in Go and not in the CHECK fails the write at run
// time. A value in the CHECK and not in Go is one no reader was written for.

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
)

var relationshipMarkCheckRE = regexp.MustCompile(
	`contact_relationship_mark_check CHECK .*relationship_mark = ANY \(ARRAY\[([^\]]*)\]`)

func TestTheRelationshipMarkVocabularyIsTheCatalogsCheck(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	match := relationshipMarkCheckRE.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("no contact_relationship_mark_check in %s — the constraint this gate reads has moved or been renamed",
			replyVerdictCatalog)
	}
	fromDB := splitCatalogTokens(string(match[1]))
	var fromGo []string
	for _, mark := range relationshipmark.Values() {
		fromGo = append(fromGo, string(mark))
	}
	sort.Strings(fromGo)
	if !equalSets(fromGo, fromDB) {
		t.Errorf("relationshipmark.Values() spells %v and the CHECK allows %v", fromGo, fromDB)
	}
}
