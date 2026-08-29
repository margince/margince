// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The notices lane: the reader's own unread notices, each carrying its one
// verb, withheld for a caller with no human behind it.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubNotices struct {
	rows []UnreadNotice
	err  error
}

func (s *stubNotices) Unread(context.Context, int) ([]UnreadNotice, error) {
	return s.rows, s.err
}

func noticesLaneService(n Notices) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, n, nil, fixedClock)
}

func TestANoticeCarriesItsOwnWordsAndItsOneVerb(t *testing.T) {
	svc := noticesLaneService(&stubNotices{rows: []UnreadNotice{
		{
			ID: ids.NewV7(), Kind: "lead_sla", Subject: "SLA breach — first response overdue",
			Body: "A lead's first response is overdue.", CreatedAt: readInstant,
		},
	}})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Notices == nil || len(*out.Notices) != 1 {
		t.Fatalf("notices carries %v, want one", out.Notices)
	}
	notice := (*out.Notices)[0]
	if notice.Title == nil || *notice.Title != "SLA breach — first response overdue" {
		t.Errorf("title = %v, want the notice's own subject", notice.Title)
	}
	if notice.Detail == nil || *notice.Detail != "A lead's first response is overdue." {
		t.Errorf("detail = %v, want the notice's body", notice.Detail)
	}
	if !slices.Contains(notice.Actions, crmcontracts.AttentionItemActions("acknowledge")) {
		t.Fatal("a notice offers no acknowledge — the one verb it exists for")
	}
}

func TestARefusedNoticesReadIsNamedAsWithheld(t *testing.T) {
	out, err := noticesLaneService(&stubNotices{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Notices != nil {
		t.Fatalf("notices = %v, want the lane withheld", out.Notices)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("notices")) {
		t.Fatal("a refused notices read was not named in lanes_omitted")
	}
}

// Everything read is an EMPTY lane — the feed looked — which the absent lane
// never promises.
func TestEverythingSeenReadsAsAnEmptyLane(t *testing.T) {
	out, err := noticesLaneService(&stubNotices{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Notices == nil || len(*out.Notices) != 0 {
		t.Fatalf("notices = %v, want an empty lane", out.Notices)
	}
}
