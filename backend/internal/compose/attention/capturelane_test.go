// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The capture_health lane: the reader's own mailbox connections needing their
// hand, withheld for a caller with no human behind it.

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubCaptureHealth struct {
	rows []CaptureConcern
	err  error
}

func (s *stubCaptureHealth) CaptureConcerns(context.Context) ([]CaptureConcern, error) {
	return s.rows, s.err
}

func captureLaneService(health CaptureHealth) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, health, nil, nil, nil, nil, nil, fixedClock)
}

func TestACaptureConcernNamesTheConditionAndTheMailbox(t *testing.T) {
	labelled := ids.NewV7()
	unlabelled := ids.NewV7()
	svc := captureLaneService(&stubCaptureHealth{rows: []CaptureConcern{
		{ConnectionID: labelled, Kind: "reauth_required", Provider: "gmail", AccountLabel: "rep@example.com"},
		{ConnectionID: unlabelled, Kind: "sync_failing", Provider: "imap"},
	}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.CaptureHealth == nil || len(*out.CaptureHealth) != 2 {
		t.Fatalf("capture_health carries %v, want two concerns", out.CaptureHealth)
	}
	if out.Counts.CaptureHealth == nil || *out.Counts.CaptureHealth != 2 {
		t.Fatalf("capture_health count = %v, want 2", out.Counts.CaptureHealth)
	}
	first := (*out.CaptureHealth)[0]
	if first.Source != crmcontracts.AttentionItemSource("capture_health") {
		t.Errorf("source = %q, want capture_health", first.Source)
	}
	if first.Kind == nil || *first.Kind != "reauth_required" {
		t.Errorf("kind = %v, want reauth_required", first.Kind)
	}
	if first.Detail == nil || *first.Detail != "rep@example.com" {
		t.Errorf("detail = %v, want the account label", first.Detail)
	}
	if first.Id != labelled.String() {
		t.Errorf("id = %q, want the connection id", first.Id)
	}
	if len(first.Actions)+len((*out.CaptureHealth)[1].Actions) != 0 {
		t.Error("a capture card promises no verbs")
	}
}

// The client names a connection with no account label from its typed provider.
// No field a reader is shown carries the provider id.
func TestAnUnlabelledConnectionShowsNoProviderID(t *testing.T) {
	item := captureItem(CaptureConcern{ConnectionID: ids.NewV7(), Kind: "sync_failing", Provider: "imap"})
	if item.Detail != nil || item.CauseLabel != nil {
		t.Errorf("detail = %v, cause_label = %v, want both absent so the client names the provider",
			item.Detail, item.CauseLabel)
	}
	if item.Connection == nil || item.Connection.Provider != "imap" {
		t.Fatalf("connection = %+v, want the provider id typed for the client to name", item.Connection)
	}
	if item.CauseRef == nil || strings.Contains(*item.CauseRef, "imap") {
		t.Errorf("cause_ref = %v, want the connection's own identity, not its provider", item.CauseRef)
	}
}

// Two unlabelled connections of one provider are two things to reconnect, so
// their identities differ even though nothing a reader sees does.
func TestTwoUnlabelledConnectionsOfOneProviderAreTwoCauses(t *testing.T) {
	a := captureItem(CaptureConcern{ConnectionID: ids.NewV7(), Kind: "disconnected", Provider: "gmail"})
	b := captureItem(CaptureConcern{ConnectionID: ids.NewV7(), Kind: "disconnected", Provider: "gmail"})
	if a.CauseRef == nil || b.CauseRef == nil || *a.CauseRef == *b.CauseRef {
		t.Errorf("cause_refs %v and %v, want two distinct identities", a.CauseRef, b.CauseRef)
	}
}

func TestACalendarConnectionIsNeverCalledAMailbox(t *testing.T) {
	calendar := captureItem(CaptureConcern{ConnectionID: ids.NewV7(), Kind: "reauth_required", Provider: "gcal", Calendar: true})
	mailbox := captureItem(CaptureConcern{ConnectionID: ids.NewV7(), Kind: "reauth_required", Provider: "gmail"})
	if calendar.Connection == nil || calendar.Connection.Medium != crmcontracts.AttentionConnectionFactsMediumCalendar {
		t.Errorf("calendar connection = %+v, want medium calendar", calendar.Connection)
	}
	if mailbox.Connection == nil || mailbox.Connection.Medium != crmcontracts.AttentionConnectionFactsMediumMailbox {
		t.Errorf("mail connection = %+v, want medium mailbox", mailbox.Connection)
	}
	row := classifySystem(calendar, fixedClock()).item
	if row.Connection == nil || row.Connection.Medium != crmcontracts.AttentionConnectionFactsMediumCalendar {
		t.Errorf("worklist row connection = %+v, want the attention item's facts carried", row.Connection)
	}
}

func TestARefusedCaptureHealthReadIsNamedAsWithheld(t *testing.T) {
	svc := captureLaneService(&stubCaptureHealth{err: apperrors.ErrPermissionDenied})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.CaptureHealth != nil {
		t.Fatalf("capture_health = %v, want the lane withheld", out.CaptureHealth)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("capture_health")) {
		t.Fatal("a refused capture_health read was not named in lanes_omitted")
	}
}

// Healthy mailboxes are an EMPTY lane: the feed looked and found nothing
// broken, which the absent lane never promises.
func TestHealthyMailboxesReadAsAnEmptyLane(t *testing.T) {
	svc := captureLaneService(&stubCaptureHealth{})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.CaptureHealth == nil || len(*out.CaptureHealth) != 0 {
		t.Fatalf("capture_health = %v, want an empty lane", out.CaptureHealth)
	}
}

func TestSilenceCardUsesItsDeliveryEpisodeAsMovement(t *testing.T) {
	delivered := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	item := captureItem(CaptureConcern{ConnectionID: ids.NewV7(), Kind: "capture_silent", Provider: "gmail", FailingSince: &delivered, SubjectMovedAt: &delivered})
	if item.SubjectMovedAt == nil || !item.SubjectMovedAt.Equal(delivered) {
		t.Fatalf("movement = %v", item.SubjectMovedAt)
	}
	if item.OccurredAt == nil || !item.OccurredAt.Equal(delivered) {
		t.Fatalf("episode start = %v", item.OccurredAt)
	}
}
