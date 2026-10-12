// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// Which Live Lists filter on a field is answered by who may find them: a list
// the asker may find is named, one they may not is only counted.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	collectionsmod "github.com/margince/margince/backend/internal/modules/collections"
	customfieldsmod "github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAListTheAskerCannotFindIsCountedButNotNamed(t *testing.T) {
	f := setupFixture(t)
	column := f.defineField(t, customfieldsmod.FieldSpec{Label: "Renewal Band Use", Type: customfieldsmod.TypeText})
	other := f.defineField(t, customfieldsmod.FieldSpec{Label: "Renewal Band Other", Type: customfieldsmod.TypeText})
	for name, sharing := range map[string]string{"Rep1's own": "private", "Everyone's": "workspace"} {
		if _, err := f.lists.CreateList(f.ctx, collectionsmod.CreateListInput{
			Name: name, EntityType: "contact", ListType: "dynamic", Sharing: sharing,
			Definition: map[string]any{"or": []any{
				map[string]any{"field": "city", "op": "exists", "value": true},
				map[string]any{"field": column, "op": "eq", "value": "q3"},
			}},
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	f.filterList(t, "On another field", other, "eq", "q3")
	noLists := testPerms
	noLists.Objects = map[string]principal.ObjectGrant{"custom_field": fullGrant}

	for who, tc := range map[string]struct {
		asker  context.Context
		named  []string
		unseen int
	}{
		"its owner":            {f.ctx, []string{"Everyone's", "Rep1's own"}, 0},
		"another seat":         {f.e.As(f.e.Rep2, nil, testPerms), []string{"Everyone's"}, 1},
		"a seat without lists": {f.e.As(f.e.Rep2, nil, noLists), nil, 2},
	} {
		got, err := f.lists.LiveListsUsingField(tc.asker, "contact", column)
		if err != nil {
			t.Fatalf("%s: %v", who, err)
		}
		if names := namesOf(got); !slices.Equal(names, tc.named) || got.UnseenCount != tc.unseen {
			t.Errorf("%s is told %v and %d unseen, want %v and %d", who, names, got.UnseenCount, tc.named, tc.unseen)
		}
	}
}

func namesOf(got crmcontracts.CustomFieldLiveLists) []string {
	var out []string
	for _, l := range got.Lists {
		out = append(out, l.Name)
	}
	return out
}
