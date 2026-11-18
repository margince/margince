// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

import (
	"context"
	"errors"
	"strings"
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

type returnsLogged struct{ rows []crmcontracts.Activity }

func (r returnsLogged) NewestLoggedTouch(context.Context, draftcore.LoggedOn, time.Time) ([]crmcontracts.Activity, error) {
	return r.rows, nil
}

// A lead with only an internal note on file is still written to as a stranger.
// The same lead with a past meeting is followed up.
func TestANoteOnlyLeadIsStillAFirstTouch(t *testing.T) {
	now := time.Now()
	bodyFor := func(kind crmcontracts.ActivityKind) string {
		t.Helper()
		row := crmcontracts.Activity{Kind: kind, OccurredAt: now.Add(-48 * time.Hour), OccurredPrecision: "instant"}
		svc := leaddraft.NewService(&leadReader{lead: lead(nil)}, &correspondence{rows: []crmcontracts.Activity{row}}, nil).
			WithLoggedTouches(returnsLogged{rows: []crmcontracts.Activity{row}})
		draft, _, err := svc.Draft(humanCtx(), someLead, leaddraft.Request{})
		if err != nil {
			t.Fatalf("drafting: %v", err)
		}
		return draft.Body
	}
	note, meeting := bodyFor(crmcontracts.ActivityKindNote), bodyFor(crmcontracts.ActivityKindMeeting)
	const firstTouch = "for the first time"
	if !strings.Contains(note, firstTouch) {
		t.Errorf("a note-only lead's draft = %q, want the first-touch opener", note)
	}
	if strings.Contains(meeting, firstTouch) {
		t.Errorf("a lead with a past meeting got the first-touch opener: %q", meeting)
	}
}
