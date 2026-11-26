// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

type newestNote struct {
	row crmcontracts.Activity
	err error
}

func (n newestNote) NewestLoggedTouch(context.Context, draftcore.LoggedOn, time.Time) ([]crmcontracts.Activity, error) {
	return []crmcontracts.Activity{n.row}, n.err
}

// The newest logged touch joins the page's rows without changing the page, a
// reader's failure stops the draft, and no reader adds nothing.
func TestTheNewestLoggedTouchJoinsThePagesRows(t *testing.T) {
	page := make([]crmcontracts.Activity, 1, 2)
	page[0].Kind = crmcontracts.ActivityKindTask
	note := crmcontracts.Activity{Kind: crmcontracts.ActivityKindNote}

	got, err := draftcore.WithNewestLogged(t.Context(), newestNote{row: note}, draftcore.LoggedOn{}, clocktest.Now(t), page)
	if err != nil || len(got) != 2 || got[1].Kind != crmcontracts.ActivityKindNote {
		t.Fatalf("rows = %v (%v), want the page and the note", got, err)
	}
	if page[:2][1].Kind == crmcontracts.ActivityKindNote {
		t.Error("the note was written into the page's own backing array")
	}
	if _, err := draftcore.WithNewestLogged(t.Context(), newestNote{err: errors.New("down")}, draftcore.LoggedOn{}, clocktest.Now(t), page); err == nil {
		t.Error("a reader's failure was swallowed")
	}
	if got, err := draftcore.WithNewestLogged(t.Context(), nil, draftcore.LoggedOn{}, clocktest.Now(t), page); err != nil || len(got) != 1 {
		t.Errorf("no reader returned %v (%v), want the page alone", got, err)
	}
}
