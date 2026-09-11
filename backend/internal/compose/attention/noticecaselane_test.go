// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The notice_case lane: the disclosure duties whose deadlines are running reach
// the one contact the case queue admits, and nobody else even learns the lane
// exists.

import (
	"context"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// theOwedContact is whose duty the card must name — a fixed id, so the assertion
// can tell "named the right contact" from "named a contact".
var theOwedContact = ids.MustParse("01a05500-0000-7000-8000-0000000000d1")

type stubNoticeCases struct {
	rows []NoticeCase
	err  error
}

func (s *stubNoticeCases) OpenDueSoonest(context.Context, int) ([]NoticeCase, error) {
	return s.rows, s.err
}

func noticeCaseLaneService(cases NoticeCases) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
		WithNoticeCases(cases))
}

func TestAnUndischargedDutyReachesTheAdminWithItsDeadline(t *testing.T) {
	overdue := readInstant.Add(-24 * time.Hour)
	svc := noticeCaseLaneService(&stubNoticeCases{rows: []NoticeCase{
		{ID: ids.NewV7(), Rule: "art14", ContactID: theOwedContact, DueAt: overdue},
		{
			ID: ids.NewV7(), Rule: "art13", ContactID: ids.NewV7(),
			DueAt: readInstant.Add(72 * time.Hour),
		},
	}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.NoticeCases == nil || len(*out.NoticeCases) != 2 ||
		out.Counts.NoticeCases == nil || *out.Counts.NoticeCases != 2 {
		t.Fatalf("lane = %v (count %v), want both open duties", out.NoticeCases, out.Counts.NoticeCases)
	}
	// The lane preserves the store's soonest-first order rather than re-sorting:
	// the overdue duty seeded FIRST is still first, and the assertions below
	// read it by position. A lane that reordered would silently put the least
	// urgent duty at the top of a bounded page.
	late := (*out.NoticeCases)[0]
	// The ARTICLE, not the state: a reader deciding what to do next needs to
	// know whether the subject knows we hold their data at all.
	if late.Kind == nil || *late.Kind != "art14" || late.DueAt == nil {
		t.Errorf("the card lost the duty's article or deadline: %+v", late)
	}
	if late.Overdue == nil || !*late.Overdue {
		t.Error("a deadline already passed is not marked overdue")
	}
	// THE CONTACT, and a verb to reach them. A notice case has no screen of its
	// own — the disclosure is sent from the contact's page — so a card carrying
	// only an article and a date would prompt an admin with nowhere to go.
	if late.Subject == nil || late.Subject.Type != "contact" {
		t.Fatalf("the card names no contact, so nobody can act on it: %+v", late)
	}
	if late.Subject.Id != openapi_types.UUID(theOwedContact) {
		t.Errorf("the card names the wrong contact: %v", late.Subject.Id)
	}
	if len(late.Actions) != 1 || late.Actions[0] != actionOpen {
		t.Errorf("the card offers %v, want the one verb that reaches the contact", late.Actions)
	}
}

func TestTheNoticeCaseLaneKeepsAbsentWithheldAndEmptyApart(t *testing.T) {
	unwired, err := noticeCaseLaneService(nil).Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling without the reader: %v", err)
	}
	// ABSENT, not empty. An installation that never looked must not report
	// "nothing owed" — that is the reading this lane exists to make impossible.
	if unwired.NoticeCases != nil || unwired.Counts.NoticeCases != nil {
		t.Error("an installation that reads no notice queue still sent the lane")
	}

	refused, err := noticeCaseLaneService(
		&stubNoticeCases{err: apperrors.ErrPermissionDenied}).Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling with a refused read: %v", err)
	}
	if refused.LanesOmitted == nil ||
		!slices.Contains(*refused.LanesOmitted, crmcontracts.AttentionLanesOmitted("notice_case")) {
		t.Errorf("a refused lane is not named in lanes_omitted: %v", refused.LanesOmitted)
	}

	clearDay, err := noticeCaseLaneService(&stubNoticeCases{}).Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling a clear lane: %v", err)
	}
	if clearDay.NoticeCases == nil || len(*clearDay.NoticeCases) != 0 {
		t.Errorf("a clear lane should be an empty list, got %v", clearDay.NoticeCases)
	}
}

// TestADueDutyReachesTheWorklist is the reach the whole item turns on: the lane
// is not merely assembled, it is CLASSIFIED, so a due case lands on the page a
// privacy admin actually reads.
//
// Asserted through Worklist rather than Assemble, because a lane the feed
// carries and the walk never classifies would leave the duty exactly as unseen
// as it was before — which is the defect this closes, one step further down.
func TestADueDutyReachesTheWorklist(t *testing.T) {
	overdue := readInstant.Add(-24 * time.Hour)
	svc := noticeCaseLaneService(&stubNoticeCases{rows: []NoticeCase{
		{ID: ids.NewV7(), Rule: "art14", DueAt: overdue},
	}})
	page, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	var found *crmcontracts.WorklistItem
	for i := range page.Queue {
		if page.Queue[i].Source == crmcontracts.WorklistItemSourceNoticeCase {
			found = &page.Queue[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("the due duty never reached the worklist: %d rows, none from notice_case",
			len(page.Queue))
	}
	if found.DueAt == nil || found.Overdue == nil || !*found.Overdue {
		t.Errorf("the row lost the deadline the whole lane exists for: %+v", found)
	}
}
