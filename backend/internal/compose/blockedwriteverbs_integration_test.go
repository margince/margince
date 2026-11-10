// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	refusedSend = "You do not have permission to send email"
	refusedTask = "You do not have permission to add tasks"
)

// readsActivities is the admin's grants with activity.create taken away, so
// the reader still sees every row and may write no activity.
func readsActivities() principal.Permissions {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["activity"] = principal.ObjectGrant{Read: true}
	return perms
}

func TestTheWorklistOffersAMailMoveOnlyToASeatThatMayWrite(t *testing.T) {
	quiet := seedQuietRelationship(t)
	e := quiet.e
	team := []ids.UUID{e.Team1}
	for name, reader := range map[string]struct {
		ctx  context.Context
		want *string
	}{
		"a full seat that may write":    {integration.OnSeat(e.As(e.Rep1, team, integration.AdminPerms), principal.SeatFull), nil},
		"a full seat without the grant": {integration.OnSeat(e.As(e.Rep1, team, readsActivities()), principal.SeatFull), new(refusedSend)},
		"a read seat holding the grant": {integration.OnSeat(e.As(e.Rep1, team, integration.AdminPerms), principal.SeatRead), new(refusedSend)},
	} {
		page := quiet.read(reader.ctx)
		if page.OptionalRelationships == nil || len(*page.OptionalRelationships) != 1 {
			t.Fatalf("%s: the quiet contact is missing: %+v", name, page.OptionalRelationships)
		}
		move := (*page.OptionalRelationships)[0].Move
		if move == nil || move.Action != crmcontracts.WorklistMoveActionDraftEmail {
			t.Fatalf("%s: the quiet contact offers no mail move: %+v", name, move)
		}
		if !sameReason(move.BlockedReason, reader.want) {
			t.Errorf("%s: blocked_reason %v, want %v", name, reasonText(move.BlockedReason), reasonText(reader.want))
		}
	}
}

func TestTheCompanyPageOffersADraftReplyOnlyToASeatThatMayWrite(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Example Company", &e.Rep1)
	subject, body, direction := "Quotation", "Our offer, as discussed.", "outbound"
	at := now.AddDate(0, 0, -10)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	}); err != nil {
		t.Fatal(err)
	}
	service := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), func() time.Time { return now })
	scan := companyscan.NewService(e.Pool, service, service, nil, nil, nil, func() time.Time { return now }, nil)
	for name, reader := range map[string]struct {
		ctx  context.Context
		want *string
	}{
		"a full seat that may write":    {integration.OnSeat(e.Admin(), principal.SeatFull), nil},
		"a full seat without the grant": {integration.OnSeat(e.As(e.AdminUser, nil, readsActivities()), principal.SeatFull), new(refusedSend)},
		"a read seat holding the grant": {integration.OnSeat(e.Admin(), principal.SeatRead), new(refusedSend)},
	} {
		page, err := service.Assemble(reader.ctx, ids.From[ids.CompanyKind](company))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		scanned, err := scan.Get(reader.ctx, ids.From[ids.CompanyKind](company))
		if err != nil {
			t.Fatalf("%s: reading the account scan: %v", name, err)
		}
		for surface, rows := range map[string][]crmcontracts.Company360Suggestion{
			"company page": derefRows(page.Suggestions), "account scan": scanned.Findings,
		} {
			action := noReplyAction(t, rows).Action
			if action.Kind != crmcontracts.Company360SuggestionActionKindDraftReply {
				t.Fatalf("%s, %s: the unanswered quotation offers %q, want draft_reply", name, surface, action.Kind)
			}
			if !sameReason(action.BlockedReason, reader.want) {
				t.Errorf("%s, %s: blocked_reason %v, want %v", name, surface, reasonText(action.BlockedReason), reasonText(reader.want))
			}
		}
	}
}

// The card is filed once per deal for every reader. Each read carries its own
// reader's answer, whichever reader filled the cache.
func TestTheDealCardOffersItsTaskOnlyToASeatThatMayWrite(t *testing.T) {
	e := setupVerdicts(t)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	team := []ids.UUID{e.Team1}
	for _, reader := range []struct {
		name string
		ctx  context.Context
		want *string
	}{
		{"a full seat that may write", integration.OnSeat(e.As(e.Rep1, team, integration.AdminPerms), principal.SeatFull), nil},
		{"a full seat without the grant", integration.OnSeat(e.As(e.Rep1, team, readsActivities()), principal.SeatFull), new(refusedTask)},
		{"a read seat holding the grant", integration.OnSeat(e.As(e.Rep1, team, integration.AdminPerms), principal.SeatRead), new(refusedTask)},
		{"the writer again, after the cache was filled", integration.OnSeat(e.As(e.Rep1, team, integration.AdminPerms), principal.SeatFull), nil},
	} {
		card, err := cards.ReadFacts(reader.ctx, ids.From[ids.DealKind](e.calm))
		if err != nil {
			t.Fatalf("%s: %v", reader.name, err)
		}
		if card.Next == nil || card.Next.Action != dealstatus.ActionCreateTask {
			t.Fatalf("%s: the calm deal offers %+v, want create_task", reader.name, card.Next)
		}
		if !sameReason(card.Next.BlockedReason, reader.want) {
			t.Errorf("%s: blocked_reason %v, want %v", reader.name, reasonText(card.Next.BlockedReason), reasonText(reader.want))
		}
	}
}

// noReplyAction is the suggestion the unanswered message raised.
func noReplyAction(t *testing.T, rows []crmcontracts.Company360Suggestion) crmcontracts.Company360Suggestion {
	t.Helper()
	for _, suggestion := range rows {
		if suggestion.Kind == crmcontracts.Company360SuggestionKindNoReply && suggestion.Action != nil {
			return suggestion
		}
	}
	t.Fatalf("the unanswered quotation raised no suggestion with an action: %+v", rows)
	return crmcontracts.Company360Suggestion{}
}

func derefRows(rows *[]crmcontracts.Company360Suggestion) []crmcontracts.Company360Suggestion {
	if rows == nil {
		return nil
	}
	return *rows
}

func sameReason(got, want *string) bool {
	return (got == nil) == (want == nil) && (got == nil || *got == *want)
}

func reasonText(reason *string) string {
	if reason == nil {
		return "<none>"
	}
	return *reason
}
