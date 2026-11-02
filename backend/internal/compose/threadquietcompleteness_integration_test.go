// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func quietCandidate(t *testing.T, o *owedEnv, entity string, id ids.UUID) bool {
	t.Helper()
	rows, err := quietScanStore(o.e.DB()).LastTouchBefore(o.judge(), o.now.Add(-7*24*time.Hour), 100, "no_activity_reminder")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.EntityType == entity && row.EntityID == id {
			return true
		}
	}
	return false
}

func TestARepairingRecentTouchCannotCreateAQuietReminder(t *testing.T) {
	for _, account := range []bool{false, true} {
		name := "direct deal"
		if account {
			name = "account through deal"
		}
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.e.GrantRole(t, o.owner, "admin")
			ctx := partySeatContext(t, o.e, o.owner)
			pat := o.contact(t, "Pat Buyer", "pat@customer.example")
			pipeline, open, _ := integration.DealFixture(t, o.e)
			var companyID *ids.CompanyID
			entity := "deal"
			if account {
				owner := ids.From[ids.UserKind](o.owner)
				company, err := o.e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Quiet account", Source: "manual", OwnerID: &owner})
				if err != nil {
					t.Fatal(err)
				}
				id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
				companyID = &id
				o.e.WsExec(t, `UPDATE company SET created_at=$2 WHERE id=$1`, company.Id, o.now.Add(-90*24*time.Hour))
				entity = "company"
			}
			createDeal := func(name string, company *ids.CompanyID) ids.UUID {
				t.Helper()
				owner := ids.From[ids.UserKind](o.owner)
				d, err := o.e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: name, PipelineID: pipeline, StageID: open, CompanyID: company, OwnerID: &owner, Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				o.e.WsExec(t, `UPDATE deal SET created_at=$2 WHERE id=$1`, d.Id, o.now.Add(-90*24*time.Hour))
				subject, at := "Discovery call", o.now.Add(-20*24*time.Hour)
				if _, _, err := o.e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "call", Subject: &subject, OccurredAt: &at, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: ids.UUID(d.Id)}}}); err != nil {
					t.Fatal(err)
				}
				return ids.UUID(d.Id)
			}
			deal := createDeal("Quiet target", companyID)
			control := createDeal("Unrelated quiet deal", nil)
			target := deal
			if companyID != nil {
				target = companyID.UUID
			}
			if !quietCandidate(t, o, entity, target) || !quietCandidate(t, o, "deal", control) {
				t.Fatal("old genuine touches did not establish quiet candidates")
			}
			rules := automation.NewAutomationStore(o.e.DB()).WithClock(func() time.Time { return o.now })
			rule, err := rules.Create(ctx, automation.CreateAutomationInput{Key: "no_activity_reminder", Name: "Quiet follow-up", Params: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			enabled := true
			if _, err := rules.Update(ctx, rule.ID, automation.UpdateAutomationInput{Enabled: &enabled}); err != nil {
				t.Fatal(err)
			}
			answer := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Current discussion", at: o.now.Add(-24 * time.Hour)})
			finishThreadRepairs(t, o.e)
			if _, err := o.e.Activities.RelinkActivity(ctx, ids.From[ids.ActivityKind](answer), activities.RelinkActivityInput{EntityType: "deal", EntityID: deal}); err != nil {
				t.Fatal(err)
			}
			if quietCandidate(t, o, entity, target) {
				t.Fatal("a genuine recent touch left a quiet candidate")
			}
			holdAnswerRepair(t, o, answer)
			if quietCandidate(t, o, entity, target) {
				t.Fatal("an unavailable recent touch created a quiet candidate")
			}
			scanner := NewTimeScannerWithClock(o.e.DB(), func() time.Time { return o.now }, slog.New(slog.DiscardHandler))
			scan := func() {
				t.Helper()
				if err := scanner.ScanWorkspace(principal.WithWorkspaceID(t.Context(), o.e.WS), o.e.WS); err != nil {
					t.Fatal(err)
				}
			}
			count := func(kind string, id ids.UUID) int {
				t.Helper()
				return o.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id=a.id WHERE a.kind='task' AND l.entity_type=$1 AND coalesce(l.deal_id,l.company_id)=$2`, kind, id)
			}
			scan()
			if count(entity, target) != 0 || count("deal", control) != 1 {
				t.Fatal("repair either created a target reminder or blocked the unrelated quiet deal")
			}
			finishThreadRepairs(t, o.e)
			scan()
			if quietCandidate(t, o, entity, target) || count(entity, target) != 0 {
				t.Fatal("published recent touch created a quiet reminder")
			}
			if _, err := o.e.Activities.ArchiveActivity(ctx, ids.From[ids.ActivityKind](answer), nil); err != nil {
				t.Fatal(err)
			}
			finishThreadRepairs(t, o.e)
			if !quietCandidate(t, o, entity, target) {
				t.Fatal("true recent-touch removal did not restore quiet eligibility")
			}
			scan()
			if count(entity, target) != 1 {
				t.Fatal("true recent-touch removal did not create one reminder")
			}
			if pat == ids.Nil {
				t.Fatal("missing correspondent")
			}
		})
	}
}
