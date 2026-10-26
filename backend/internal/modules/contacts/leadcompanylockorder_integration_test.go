// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A promotion locks the lead's linked company before the lead, as the link and
// a company merge do. It reads that company without a lock, so a link can land
// in between; the promotion then starts again with the company first.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withCompanyGrants is the env's actor with the company and edge grants a link
// and the employment its promotion carries need.
func (e *promoteConsentEnv) withCompanyGrants() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	all := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"lead": all, "contact": all, "company": all, "relationship": all, "custom_field": all,
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// beginOn opens a transaction on a connection of its own, with its backend pid.
func beginOn(t *testing.T) (pgx.Tx, int) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("close the connection: %v", err)
		}
	})
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { endTx(t, tx) })
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return tx, pid
}

// The promotion reads no company, parks on the contact, and a link then lands.
// A company merge's stand-in holds the company and next wants the lead. A
// promotion that took the company after its lead would close a cycle with it.
func TestAPromotionTakesALinkThatLandedBeforeItsLeadLockCompanyFirst(t *testing.T) {
	e := setupPromoteConsent(t)
	ctx := context.Background()
	as := e.withCompanyGrants()
	company, err := e.store.CreateCompany(as, CreateCompanyInput{DisplayName: "Papa Plastics", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	contact := e.createContactWithEmail(t, "Lena Lead", "lena@papa.test")
	created, _, err := e.store.CreateLead(as, CreateLeadInput{
		FullName: new("Lena Lead"), Email: new("lena@papa.test"), CompanyName: new("Papa Plastics"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	lead := ids.From[ids.LeadKind](ids.UUID(created.Id))

	holder, holderPid := beginOn(t)
	if _, err := holder.Exec(ctx, `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, contact); err != nil {
		t.Fatal(err)
	}
	promoted := make(chan promoteResult, 1)
	go func() {
		out, err := e.store.QualifyLead(as, lead, PromoteLeadInput{Trigger: "human_qualify"})
		promoted <- promoteResult{out: out, err: err}
	}()
	if got, finished := waitUntilBlocked(t, holder, holderPid, promoted); finished {
		t.Fatalf("the promote finished (%v) without parking on the contact", got.err)
	}
	if _, err := e.store.LinkLeadCompany(as, lead, companyID); err != nil {
		t.Fatalf("link while the promote is parked: %v", err)
	}

	merger, mergerPid := beginOn(t)
	if _, err := merger.Exec(ctx, `SET LOCAL lock_timeout = '10s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := merger.Exec(ctx, `SELECT id FROM company WHERE id = $1 FOR UPDATE`, companyID); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	probe, _ := beginOn(t)
	if got, finished := waitUntilBlocked(t, probe, mergerPid, promoted); finished {
		t.Fatalf("the promote finished (%v) without waiting on the held company", got.err)
	}
	if _, err := merger.Exec(ctx, `SELECT id FROM lead WHERE id = $1 FOR UPDATE`, lead); err != nil {
		t.Fatalf("the company holder could not lock the lead: %v (deadlock=%t)", err, storekit.IsDeadlock(err))
	}
	if err := merger.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := <-promoted
	assertLandedOn(t, got, contact)
	var employed bool
	if err := probe.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM relationship WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment')`,
		contact, companyID).Scan(&employed); err != nil {
		t.Fatal(err)
	}
	if !employed {
		t.Error("the promotion carried no employment at the company linked while it waited")
	}
}
