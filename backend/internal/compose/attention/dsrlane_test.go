// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The dsr lane: the requests whose legal clocks are running reach the one
// person the case queue admits, and nobody else even learns the lane exists.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubDSRs struct {
	rows []DSRCase
	err  error
}

func (s *stubDSRs) OpenDueSoonest(context.Context, int) ([]DSRCase, error) {
	return s.rows, s.err
}

func dsrLaneService(dsrs DSRs) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, dsrs, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

func TestAnOpenRequestReachesTheAdminWithItsDeadline(t *testing.T) {
	overdue := readInstant.Add(-24 * time.Hour)
	svc := dsrLaneService(&stubDSRs{rows: []DSRCase{
		{ID: ids.NewV7(), Kind: "erasure", DueAt: overdue},
		{ID: ids.NewV7(), Kind: "access", DueAt: readInstant.Add(72 * time.Hour)},
	}})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Dsr == nil || len(*out.Dsr) != 2 || out.Counts.Dsr == nil || *out.Counts.Dsr != 2 {
		t.Fatalf("lane = %v (count %v), want both open requests", out.Dsr, out.Counts.Dsr)
	}
	late := (*out.Dsr)[0]
	if late.Kind == nil || *late.Kind != "erasure" || late.DueAt == nil {
		t.Errorf("the card lost the request's kind or deadline: %+v", late)
	}
	if late.Overdue == nil || !*late.Overdue {
		t.Error("a deadline already passed is not marked overdue")
	}
	// No verbs and no subject: the request is the card's whole subject, and
	// working the case lives on the case queue's own screen.
	if len(late.Actions) != 0 || late.Subject != nil {
		t.Errorf("the card promises interaction it cannot honour: %+v", late)
	}
}

func TestTheDSRLaneKeepsAbsentWithheldAndEmptyApart(t *testing.T) {
	unwired, err := dsrLaneService(nil).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling without the reader: %v", err)
	}
	if unwired.Dsr != nil || unwired.Counts.Dsr != nil {
		t.Error("an installation that reads no case queue still sent the lane")
	}

	refused, err := dsrLaneService(&stubDSRs{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling with a refused read: %v", err)
	}
	if refused.LanesOmitted == nil || !slices.Contains(*refused.LanesOmitted, crmcontracts.AttentionLanesOmitted("dsr")) {
		t.Errorf("a refused lane is not named in lanes_omitted: %v", refused.LanesOmitted)
	}

	clearDay, err := dsrLaneService(&stubDSRs{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling a clear lane: %v", err)
	}
	if clearDay.Dsr == nil || len(*clearDay.Dsr) != 0 {
		t.Errorf("a clear lane should be an empty list, got %v", clearDay.Dsr)
	}
}
