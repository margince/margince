// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type askedLogged struct {
	on  draftcore.LoggedOn
	err error
}

func (a *askedLogged) NewestLoggedTouch(_ context.Context, on draftcore.LoggedOn, _ time.Time) ([]crmcontracts.Activity, error) {
	a.on = on
	return nil, a.err
}

// The lead's timeline is bounded, so the draft asks for the lead's newest
// logged touch on its own, and a failed read stops it.
func TestALeadDraftReadsTheNewestLoggedTouchOfItsLead(t *testing.T) {
	reader := &askedLogged{}
	svc := leaddraft.NewService(&leadReader{lead: lead(nil)}, &correspondence{}, nil).WithLoggedTouches(reader)

	if _, _, err := svc.Draft(humanCtx(), someLead, leaddraft.Request{}); err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if reader.on.RecordType != "lead" || reader.on.ID != someLead.UUID {
		t.Errorf("the draft asked about %+v, want the lead", reader.on)
	}
	reader.err = errors.New("down")
	if _, _, err := svc.Draft(humanCtx(), someLead, leaddraft.Request{}); err == nil {
		t.Error("a failed logged-touch read still produced a draft")
	}
}
