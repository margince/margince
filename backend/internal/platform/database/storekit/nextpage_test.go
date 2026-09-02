// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The page-building half of "a position that cannot be written down".
//
// opaquecursor_test.go holds the encoder half: a year-294276 instant is an
// error rather than an empty token. This is what the LIST does with that
// answer, which is where the damage was — HasMore: true beside an empty
// NextCursor tells a client there is another page and hands them nothing to
// fetch it with. Silent on the server, permanent for that list.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// beyondJSON is an instant Postgres stores and JSON cannot write: timestamptz
// reaches year 294276, and time.Time refuses to marshal outside 0000-9999.
var beyondJSON = time.Date(294276, 1, 1, 0, 0, 0, 0, time.UTC)

func TestAPageWhoseCursorWillNotMintIsAnErrorNotAnEmptyPromise(t *testing.T) {
	sorted, err := ParseListSort(sortSpec("full_name"), testVocab)
	if err != nil {
		t.Fatalf("building a non-default sort: %v", err)
	}
	key := "Weber GmbH"
	for name, mint := range map[string]func() (Page, error){
		"the house sort":     func() (Page, error) { return nextPage(nil, nil, beyondJSON, ids.NewV7()) },
		"a non-default sort": func() (Page, error) { return nextPage(sorted, &key, beyondJSON, ids.NewV7()) },
	} {
		t.Run(name, func(t *testing.T) {
			page, err := mint()
			if err == nil {
				t.Fatalf("page = %+v with no error — a client told there is more and handed %q "+
					"to fetch it with can ask for that page forever", page, page.NextCursor)
			}
			if page.HasMore || page.NextCursor != "" {
				t.Errorf("page = %+v alongside the error, want nothing: the flag and the token "+
					"are set together or not at all", page)
			}
		})
	}
}

// AND AN ORDINARY ROW STILL GETS BOTH, so the refusal above is not the only
// thing this can do.
func TestAnOrdinaryRowGetsTheFlagAndTheToken(t *testing.T) {
	page, err := nextPage(nil, nil, time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC), ids.NewV7())
	if err != nil {
		t.Fatalf("nextPage: %v", err)
	}
	if !page.HasMore || page.NextCursor == "" {
		t.Errorf("page = %+v, want has_more with the token that continues it", page)
	}
}
