// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A German reader opens the account and the worker reads it in the seat's own
// language. The read is current, because a task Margince wrote is named in the
// scan's language on both sides.
func TestAScanOpenedInGermanAndReadByTheWorkerIsCurrent(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace", databaseNow(t, e))
	subject := systemtext.Literal("automation.new_lead")
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual",
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("automation.new_lead")},
		Links:      []activities.ActivityLinkInput{{EntityType: "company", EntityID: company.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, databaseClock(t))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	german := systemtext.WithLocale(rep, textlang.German)

	if _, err := svc.Ensure(german, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("the open queued %d reads, want 1", len(queued))
	}
	if err := svc.Run(principal.WithCorrelationID(rep, queued[0].ScanID), queued[0].ScanID, company); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := svc.Get(german, company)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != crmcontracts.CompanyScanStateDone || (got.Stale != nil && *got.Stale) {
		t.Errorf("the German reader's scan is %q stale=%v, want a current read", got.State, got.Stale)
	}
}
