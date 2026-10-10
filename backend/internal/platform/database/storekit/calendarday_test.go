// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// One range answers every way a date reaches a date column: a body value, a
// query-parameter filter and a continuation token's sort key.
func TestCustomDateKeepsOneStorableRangeAcrossItsEntryPoints(t *testing.T) {
	c := col("cf_renewal", fieldcatalog.TypeDate)
	for _, day := range []string{"0000-01-01", "0001-01-01", "9999-12-31"} {
		if _, ok := SQLValue(c, day); ok {
			t.Errorf("SQLValue bound %q", day)
		}
		if err := checkListFilterValue(c, day); err == nil {
			t.Errorf("checkListFilterValue accepted %q", day)
		}
	}
	for _, day := range []string{"0001-01-02", "2026-10-10", "9999-12-30"} {
		if _, ok := SQLValue(c, day); !ok {
			t.Errorf("SQLValue dropped %q", day)
		}
		if err := checkListFilterValue(c, day); err != nil {
			t.Errorf("checkListFilterValue refused %q: %v", day, err)
		}
	}
}

func TestACursorKeyInYearZeroIsMalformed(t *testing.T) {
	vocab := SortVocabulary(nil, []fieldcatalog.Column{{Name: "cf_renewal", Type: fieldcatalog.TypeDate}})
	s, err := ParseListSort(context.Background(), new("cf_renewal"), vocab, noArgs)
	if err != nil {
		t.Fatal(err)
	}
	key := "0000-01-01"
	token := mustEncodeOpaque(t, Cursor{
		CreatedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), ID: ids.NewV7(),
		SortField: "cf_renewal", SortKey: &key,
	})
	arg, _ := keysetArgs()
	_, err = s.KeysetClause(token, arg)
	if _, ok := errors.AsType[*MalformedCursorError](err); !ok {
		t.Fatalf("err = %v, want MalformedCursorError", err)
	}
}
