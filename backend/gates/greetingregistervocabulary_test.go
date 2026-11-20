// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// The greeting register is spelled twice: greetingregister.Values() and the
// column's CHECK. A value in Go and not in the CHECK fails the write at run
// time. A value in the CHECK and not in Go is one no reader was written for.

import (
	"regexp"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
)

var greetingRegisterCheckRE = regexp.MustCompile(
	`contact_greeting_register_check CHECK .*greeting_register = ANY \(ARRAY\[([^\]]*)\]`)

func TestTheGreetingRegisterVocabularyIsTheCatalogsCheck(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	match := greetingRegisterCheckRE.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("no contact_greeting_register_check in %s: the constraint this gate reads has moved or been renamed",
			replyVerdictCatalog)
	}
	fromDB := splitCatalogTokens(string(match[1]))
	var fromGo []string
	for _, register := range greetingregister.Values() {
		fromGo = append(fromGo, string(register))
	}
	sort.Strings(fromGo)
	if !equalSets(fromGo, fromDB) {
		t.Errorf("greetingregister.Values() spells %v and the CHECK allows %v", fromGo, fromDB)
	}
}
