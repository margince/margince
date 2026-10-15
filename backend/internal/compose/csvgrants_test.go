// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// grantNames spells each need as object:action, in the order grantsFor gave.
func grantNames(needs []grantNeed) []string {
	names := make([]string, 0, len(needs))
	for _, need := range needs {
		names = append(names, need.Object+":"+string(need.Action))
	}
	return names
}

func TestGrantsForATaggedContactsFileAreItsWritesAndTheTagGrants(t *testing.T) {
	parts := []migration.PartMapping{{Object: migration.ObjectContact, Fields: map[string]string{"Tags": csvTargetTags}}}
	got := grantNames(grantsFor(parts, tagGrants{applies: true, createsNew: true}))
	want := []string{"contact:create", "contact:update", "tag:read", "tag:create"}
	if !slices.Equal(got, want) {
		t.Fatalf("grantsFor = %v, want %v", got, want)
	}
}

func TestGrantsForNewTagsAloneIsTagCreate(t *testing.T) {
	needs := grantsFor(nil, tagGrants{createsNew: true})
	if got := grantNames(needs); !slices.Equal(got, []string{"tag:create"}) {
		t.Fatalf("grantsFor(new tags only) = %v, want only tag:create", got)
	}
	if needs[0].Action != principal.ActionCreate || !strings.Contains(needs[0].Why.English(), "create tags") {
		t.Fatalf("need = %+v, want its reason to name creating tags", needs[0])
	}
}

func TestGrantReasonsNameTheFileInWords(t *testing.T) {
	needs := grantsFor([]migration.PartMapping{{Object: migration.ObjectCompany}}, tagGrants{})
	if len(needs) != 2 || needs[0].Why.English() != "You cannot create companies, so the companies file is skipped." {
		t.Fatalf("needs = %+v, want the create reason in words first", needs)
	}
	if strings.Contains(needs[1].Why.English(), "tag") {
		t.Fatalf("update reason %q names tags, but this file maps no tags column", needs[1].Why.English())
	}
}
