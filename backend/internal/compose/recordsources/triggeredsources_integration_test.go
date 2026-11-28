// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOrdinaryEditsPublishTheSourceTheTriggerStored(t *testing.T) {
	for _, table := range []string{"contact", "company", "deal", "offer", "project", "lead"} {
		t.Run(table, func(t *testing.T) {
			o := seedOlder(t)
			ctx := o.e.Admin()
			if table == "offer" {
				actor, ok := principal.Actor(ctx)
				if !ok {
					t.Fatal("missing editor")
				}
				actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
				actor.Permissions.Objects["offer"] = principal.ObjectGrant{Read: true, Update: true}
				ctx = principal.WithActor(ctx, actor)
			}
			var id ids.UUID
			old, want := "", ""
			var err error
			switch table {
			case "contact":
				old, want = "gmail:msg-41", "capture"
				id = olderParty(t, o.owner, table, old, nil)
				_, err = o.e.Contacts.UpdateContact(ctx, ids.From[ids.ContactKind](id), contacts.UpdateContactInput{FullName: new("Changed contact")})
			case "company":
				old, want = "coldstart", "automation"
				id = olderParty(t, o.owner, table, old, nil)
				_, err = o.e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{DisplayName: new("Changed company")})
			case "deal":
				old, want = "agent:dealscout", "automation"
				id = o.deal(old)
				_, err = o.e.Deals.UpdateDeal(ctx, ids.From[ids.DealKind](id), deals.UpdateDealInput{Name: new("Changed deal")})
			case "offer":
				old, want = "seed", "api"
				id = o.offer(o.e.SeedDeal(t, "Offer deal", o.pipeline, o.stage, nil), "A-1", old)
				_, _, err = o.e.Deals.UpdateOffer(ctx, ids.From[ids.OfferKind](id), deals.UpdateOfferInput{IntroText: new("Changed introduction")})
			case "project":
				old, want = "mcp", "manual"
				company := o.e.SeedCompany(t, "Project account", nil)
				id = integration.SeedBeforeSourceCheck(t, o.owner, "project", `INSERT INTO project(id,name,key,phase,company_id,source,captured_by) VALUES($1,'Older project','SOURCE1','delivering',$2,$3,'human:old')`, company, old)
				_, err = o.e.Projects.UpdateProject(ctx, ids.From[ids.ProjectKind](id), projects.UpdateProjectInput{Name: new("Changed project")})
			case "lead":
				old, want = "legacy-producer", "api"
				id = integration.SeedBeforeSourceCheck(t, o.owner, "lead", `INSERT INTO lead(id,full_name,source,captured_by) VALUES($1,'Older Lead',$2,'human:old')`, old)
				_, err = o.e.Contacts.UpdateLead(ctx, ids.From[ids.LeadKind](id), contacts.UpdateLeadInput{FullName: new("Changed lead")})
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := o.source(table, id); got != want {
				t.Fatalf("stored source %q, want %q", got, want)
			}
			if old == want {
				if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND before->>'source' IS DISTINCT FROM after->>'source'`, id); n != 0 {
					t.Fatalf("invented %d source changes", n)
				}
				return
			}
			assertStoredSourcePublished(t, o.e, ctx, table, id, old, want)
			if table == "contact" || table == "company" {
				system := "gmail"
				if table == "company" {
					system = "coldstart"
				}
				if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND after->>'source'=$2 AND after->>'source_system'=$3`, id, want, system); n != 1 {
					t.Fatalf("source system missing from %d audit rows", n)
				}
			}
		})
	}
}

func assertStoredSourcePublished(t *testing.T, e *integration.Env, ctx context.Context, table string, id ids.UUID, before, after string) {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing editing actor")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND before->>'source'=$2 AND after->>'source'=$3 AND actor_id=$4`, id, before, after, actor.ID); n != 1 {
		t.Fatalf("%s: %d source audit rows, want one signed by the editor", table, n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id=(o.envelope->'trace'->>'audit_log_id')::uuid WHERE a.entity_id=$1 AND a.before->>'source'=$2 AND o.envelope->>'type'=$3 AND coalesce(o.envelope->'payload'->'changed_fields'->>'source',o.envelope->'payload'->>'source')=$4`, id, before, table+".updated", after); n != 1 {
		t.Fatalf("%s: %d events carry the stored source, want one", table, n)
	}
}

func TestAnActivityClockPublishesTheContactsTriggeredSourceChange(t *testing.T) {
	o := seedOlder(t)
	contact := olderParty(t, o.owner, "contact", "gmail:clock-41", nil)
	ctx := o.e.Admin()
	_, _, err := o.e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "meeting", Subject: new("Discuss delivery"), MeetingStatus: new("held"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	assertStoredSourcePublished(t, o.e, ctx, "contact", contact, "gmail:clock-41", "capture")
}

func TestARolledBackEditKeepsTheLegacySourceAndWritesNoSourceEvent(t *testing.T) {
	o := seedOlder(t)
	company := olderParty(t, o.owner, "company", "coldstart", nil)
	abort := errors.New("abort the edit")
	ctx := o.e.Admin()
	err := database.WithWriteTx(ctx, o.e.Pool, func(joined context.Context) error {
		if _, err := o.e.Contacts.UpdateCompany(joined, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{DisplayName: new("Rolled back")}); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if got := o.source("company", company); got != "coldstart" {
		t.Fatalf("source survived rollback as %q", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, company); n != 0 {
		t.Fatalf("%d audit rows survived rollback", n)
	}
	if _, err := o.e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{DisplayName: new("Committed")}); err != nil {
		t.Fatal(err)
	}
	assertStoredSourcePublished(t, o.e, ctx, "company", company, "coldstart", "automation")
}

func TestABeforeCommitFailureRollsBackTheSourceAuditAndEvent(t *testing.T) {
	o := seedOlder(t)
	company := olderParty(t, o.owner, "company", "coldstart", nil)
	abort := errors.New("cannot finish the edit")
	ctx := o.e.Admin()
	err := database.WithWriteTx(ctx, o.e.Pool, func(joined context.Context) error {
		if _, err := o.e.Contacts.UpdateCompany(joined, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{DisplayName: new("Abort at commit")}); err != nil {
			return err
		}
		return o.e.DB().Tx(joined, func(tx pgx.Tx) error {
			if !database.BeforeCommit(tx, func(context.Context, pgx.Tx) error { return abort }) {
				return errors.New("missing transaction owner")
			}
			return nil
		})
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if got := o.source("company", company); got != "coldstart" {
		t.Fatalf("source survived a failed commit as %q", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, company); n != 0 {
		t.Fatalf("%d audit rows survived the failed commit", n)
	}
}
