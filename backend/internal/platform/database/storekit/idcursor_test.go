// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnIDCursorContinuesOnlyThePagingThatMintedIt(t *testing.T) {
	deals, err := CursorScope("deals", map[string]any{"field": "stage", "op": "eq", "value": "open"})
	if err != nil {
		t.Fatal(err)
	}
	contacts, err := CursorScope("contacts", map[string]any{"field": "stage", "op": "eq", "value": "open"})
	if err != nil {
		t.Fatal(err)
	}
	last := ids.NewV7()
	page, err := IDPage(deals, []ids.UUID{ids.NewV7(), last}, true)
	if err != nil || !page.HasMore {
		t.Fatalf("a page with more answered %+v, %v", page, err)
	}
	after, err := DecodeIDCursor(deals, page.NextCursor)
	if err != nil || after == nil || *after != last {
		t.Fatalf("the cursor read back %v, %v; want the page's last id %v", after, err, last)
	}
	var malformed *MalformedCursorError
	for name, token := range map[string]string{
		"another paging's cursor": page.NextCursor,
		"a bare record id":        last.String(),
		"not a cursor":            "x",
	} {
		scope := contacts
		if name != "another paging's cursor" {
			scope = deals
		}
		if _, err := DecodeIDCursor(scope, token); !errors.As(err, &malformed) {
			t.Errorf("%s was read as a position: %v", name, err)
		}
	}
	if after, err := DecodeIDCursor(deals, ""); after != nil || err != nil {
		t.Errorf("no cursor read %v, %v; want the first page", after, err)
	}
	if page, err := IDPage(deals, []ids.UUID{last}, false); err != nil || page.HasMore || page.NextCursor != "" {
		t.Errorf("the last page answered %+v, %v", page, err)
	}
}

func TestACursorScopeReadsTheSameForTheSameFilter(t *testing.T) {
	a, errA := CursorScope("deals", map[string]any{"op": "eq", "field": "stage"})
	b, errB := CursorScope("deals", map[string]any{"field": "stage", "op": "eq"})
	if errA != nil || errB != nil || a != b {
		t.Fatalf("one filter written in two key orders gave scopes %q and %q (%v, %v)", a, b, errA, errB)
	}
}
