// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"regexp"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The history statement spells each change in SQL, where the contract's enum
// cannot reach it. So each word it can answer is held against that enum here.
func TestEveryChangeARevisionNamesIsOneTheContractKnows(t *testing.T) {
	words := regexp.MustCompile(`THEN '([a-z_]+)' END`).FindAllStringSubmatch(revisionChangesSQL, -1)
	if len(words) == 0 {
		t.Fatal("revisionChangesSQL names no change; the pattern no longer reads it")
	}
	for _, word := range words {
		if !crmcontracts.ListHistoryEntryChanges(word[1]).Valid() {
			t.Errorf("revisionChangesSQL names %q, which ListHistoryEntry.changes does not list", word[1])
		}
	}
}
