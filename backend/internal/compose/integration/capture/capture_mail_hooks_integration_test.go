// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A unit's two mail-capture hooks through the production sink: what they may
// change, what core records, and what a colleague's decision keeps out of reach.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func registerMailHooks(t *testing.T, hooks extension.MailHooks) {
	t.Helper()
	if hooks.Reasons == nil {
		hooks.Reasons = []string{"credit_hold", "names_reporting", "other_account", "stale", "pointing_elsewhere", "purchase_order", "kept"}
	}
	unit := extension.Extension{Name: "deal-rules", Version: "0.0.1", Description: "A unit composed by a test.", MailHooks: hooks}
	if err := compose.RegisterExtensions([]extension.Extension{unit}, nil, nil); err != nil {
		t.Fatalf("registering the unit: %v", err)
	}
	t.Cleanup(func() {
		if err := compose.RegisterExtensions(nil, nil, nil); err != nil {
			t.Errorf("clearing the unit: %v", err)
		}
	})
}

// A deal hook drops the only open deal, so the mail files nowhere.
func TestADealHookCanKeepAMailOffTheOnlyOpenDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	var calls atomic.Int32
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		calls.Add(1)
		return extension.MailDeal{Outcome: "none", Reason: "credit_hold"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook1@acme.example", "", "invoice"))
	wantDeals(t, e, "hook1@acme.example")
	if calls.Load() == 0 {
		t.Fatal("the hook was never called")
	}
	wantFiling(t, e, activityOf(t, e, "hook1@acme.example"), "deal-rules", "none", "credit_hold")
}

// A deal hook names a deal. The link lands and its audit evidence names the
// unit and the reason. A colleague then declines it: the hook cannot bring the
// deal back, and the next capture files the other deal by core's own rule.
func TestADealHookDealDoesNotSurviveAColleaguesDecline(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	operations := w.deal(t, "Acme operations")
	chosen := w.deal(t, "Acme reporting")
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "linked", Deal: chosen.String(), Reason: "names_reporting"}, nil
	}})
	raw := mailFrom("alice@acme.example", "hook2@acme.example", "", "reporting question")
	env.sync(t, raw)
	wantDeals(t, e, "hook2@acme.example", chosen)
	mail := activityOf(t, e, "hook2@acme.example")
	wantFiling(t, e, mail, "deal-rules", "linked", "names_reporting")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = '`+mail.String()+`'
		AND evidence::text LIKE '%deal-rules%' AND evidence::text LIKE '%names_reporting%'`); n != 1 {
		t.Fatalf("%d audit rows name the unit and its reason, want 1", n)
	}

	if status, _ := decline(w.ctx, t, e, mail, dealBody(chosen)); status != http.StatusOK {
		t.Fatalf("the decline answered %d", status)
	}
	wantDeals(t, e, "hook2@acme.example")
	// Core's own rule files the other deal. The declined one stays off.
	env.sync(t, raw)
	wantDeals(t, e, "hook2@acme.example", operations)
}

// A parties hook that names a company core did not derive moves the rule to
// that company's deal. One that names a company that does not exist is ignored.
func TestAPartiesHookAdjustsTheCompaniesTheRuleReads(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	other := w.at(t, "Elsewhere")
	elsewhere := other.deal(t, "Elsewhere rollout")
	registerMailHooks(t, extension.MailHooks{Parties: func(_ context.Context, _ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		p.Companies = []string{other.customer.String()}
		p.Reason = "other_account"
		return p, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook3@acme.example", "", "hello"))
	wantDeals(t, e, "hook3@acme.example", elsewhere)

	registerMailHooks(t, extension.MailHooks{Parties: func(_ context.Context, _ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		p.Companies = []string{ids.NewV7().String()}
		p.Reason = "stale"
		return p, nil
	}})
	beta := w.at(t, "Beta")
	beta.employee(t, "bob@beta.example", beta.customer)
	only := beta.deal(t, "Beta rollout")
	env.sync(t, mailFrom("bob@beta.example", "hook4@beta.example", "", "hello"))
	wantDeals(t, e, "hook4@beta.example", only)
}

// A deal hook cannot name a deal of a company the mail has nothing to do with.
func TestADealHookCannotReachADealOfAnotherCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	foreign := w.at(t, "Elsewhere").deal(t, "Elsewhere rollout")
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "linked", Deal: foreign.String(), Reason: "pointing_elsewhere"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook5@acme.example", "", "hello"))
	if slices.Contains(dealsOf(t, e, "hook5@acme.example"), foreign) {
		t.Fatal("the mail was filed on a deal of another company")
	}
}

// A deal hook reads the mail's text, looks the reference in it up in the
// unit's own table and links that deal. The same Lookup cannot write.
func TestADealHookReadsTheBodyAndLooksARefUpInTheUnitsOwnData(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	chosen := w.deal(t, "Acme reporting")
	owner := integration.OwnerConn(t)
	for _, statement := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`CREATE TABLE ext.ext_deal_rules_ref (ref text PRIMARY KEY, deal uuid NOT NULL)`,
		`GRANT SELECT, INSERT ON ext.ext_deal_rules_ref TO ext_runtime`,
		`INSERT INTO ext.ext_deal_rules_ref VALUES ('PO-7731', '` + chosen.String() + `')`,
	} {
		if _, err := owner.Exec(context.Background(), statement); err != nil {
			t.Fatalf("preparing the unit's table (%s): %v", statement, err)
		}
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE ext.ext_deal_rules_ref`); err != nil {
			t.Errorf("removing the unit's table: %v", err)
		}
	})
	var writeErr atomic.Value
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, m extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		var deal string
		if err := m.Lookup(`SELECT deal::text FROM ext.ext_deal_rules_ref WHERE $1 LIKE '%' || ref || '%'`, m.Body).Scan(&deal); err != nil {
			return d, err
		}
		if err := m.Lookup(`INSERT INTO ext.ext_deal_rules_ref VALUES ('X', $1) RETURNING ref`, deal).Scan(new(string)); err != nil {
			writeErr.Store(err.Error())
		}
		return extension.MailDeal{Outcome: "linked", Deal: deal, Reason: "purchase_order"}, nil
	}})
	env.sync(t, bytes.Replace(mailFrom("alice@acme.example", "hook6@acme.example", "", "an order"), []byte("hello"), []byte("our order PO-7731 please"), 1))
	wantDeals(t, e, "hook6@acme.example", chosen)
	if writeErr.Load() == nil {
		t.Fatal("the lookup accepted a write")
	}
}

// A hook that fails decides nothing. Core does not fall back to its own answer:
// the mail stays unfiled, the trace says which unit failed, and the next capture
// files it once the hook works again.
func TestAFailedHookLeavesTheMailUndecidedAndItIsFiledOnceTheHookWorks(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return d, errors.New("the ledger is down")
	}})
	raw := mailFrom("alice@acme.example", "hook7@acme.example", "", "invoice")
	env.sync(t, raw)
	wantDeals(t, e, "hook7@acme.example")
	wantFiling(t, e, activityOf(t, e, "hook7@acme.example"), "deal-rules", "failed", "")

	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return d, nil
	}})
	env.sync(t, raw)
	wantDeals(t, e, "hook7@acme.example", only)
}

// A change that names no reason the unit declared is ignored, and says so.
func TestADealHookChangeWithoutADeclaredReasonIsIgnored(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "none", Reason: "not declared"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook8@acme.example", "", "invoice"))
	wantDeals(t, e, "hook8@acme.example", only)
	wantFiling(t, e, activityOf(t, e, "hook8@acme.example"), "deal-rules", "ignored", "not declared")
}

// A parties hook cannot name a company the mailbox owner may not see. The
// answer is ignored, and no deal of that company is reached through it.
func TestAPartiesHookCannotNameACompanyTheMailboxOwnerCannotSee(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	acme := w.deal(t, "Acme rollout")
	hidden := w.at(t, "Hidden")
	hiddenDeal := hidden.deal(t, "Hidden rollout")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE company SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden.customer, e.Rep2)
		return err
	}); err != nil {
		t.Fatalf("hiding the company: %v", err)
	}
	registerMailHooks(t, extension.MailHooks{Parties: func(_ context.Context, _ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		p.Companies = []string{hidden.customer.String()}
		p.Reason = "other_account"
		return p, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook9@acme.example", "", "hello"))
	wantDeals(t, e, "hook9@acme.example", acme)
	if slices.Contains(dealsOf(t, e, "hook9@acme.example"), hiddenDeal) {
		t.Fatal("the mail was filed on a deal of a company its mailbox owner cannot see")
	}
	wantFiling(t, e, activityOf(t, e, "hook9@acme.example"), "core", "linked", "sole_open_deal")
}

// wantFiling asserts what the mail's capture trace says about who filed it.
func wantFiling(t *testing.T, e *integration.SearchEnv, mail ids.UUID, actor, result, reason string) {
	t.Helper()
	if n := countRows(t, e, `SELECT count(*) FROM capture_trace WHERE activity_id = '`+mail.String()+`'
		AND filing->>'actor' = '`+actor+`' AND filing->>'result' = '`+result+`'
		AND coalesce(filing->>'reason', '') = '`+reason+`'`); n < 1 {
		t.Fatalf("the trace does not say %s %s the mail (reason %q)", actor, result, reason)
	}
}
