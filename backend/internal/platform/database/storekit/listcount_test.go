// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"
)

func TestAListOffsetIsRefusedBesideACursorAndOutsideItsRange(t *testing.T) {
	cursor := "token"
	for _, c := range []struct {
		name   string
		offset *int
		cursor *string
		want   int
		code   string
	}{
		{"absent", nil, nil, 0, ""},
		{"absent beside a cursor", nil, &cursor, 0, ""},
		{"an empty cursor is no cursor", new(5), new(""), 5, ""},
		{"the cap", new(MaxListOffset), nil, MaxListOffset, ""},
		{"past the cap", new(MaxListOffset + 1), nil, 0, "out_of_range"},
		{"negative", new(-1), nil, 0, "out_of_range"},
		{"beside a cursor", new(0), &cursor, 0, "offset_with_cursor"},
	} {
		got, err := ListOffset(c.offset, c.cursor)
		var refused *OffsetError
		switch {
		case c.code == "" && (err != nil || got != c.want):
			t.Errorf("%s: ListOffset = %d, %v; want %d", c.name, got, err, c.want)
		case c.code != "" && (!errors.As(err, &refused) || refused.Code != c.code):
			t.Errorf("%s: err = %v, want an OffsetError coded %s", c.name, err, c.code)
		}
	}
}

// The total a page states without a COUNT: nil tx, so a case that reached the
// database would panic.
func TestAPageThatEndsTheListIsItsOwnTotal(t *testing.T) {
	for _, c := range []struct {
		offset, fetched, want int
	}{{0, 0, 0}, {0, 3, 3}, {10, 1, 11}, {10, 3, 13}} {
		got, err := PageTotal(context.Background(), nil, ListCount{}, c.offset, c.fetched, 3)
		if err != nil || got != c.want {
			t.Errorf("offset %d, %d fetched: total = %d, %v; want %d", c.offset, c.fetched, got, err, c.want)
		}
	}
}

func TestAnOffsetPageAsksForOneRowPastItsSize(t *testing.T) {
	if got := PageWindow(25, 0); got != " LIMIT 26" {
		t.Errorf("a first page window = %q", got)
	}
	if got := PageWindow(25, 50); got != " LIMIT 26 OFFSET 50" {
		t.Errorf("an offset page window = %q", got)
	}
}
