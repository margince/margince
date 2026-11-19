// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A unit's two mail-capture hooks through the production sink: what they may
// change, what core records, and what a person's decision keeps out of reach.

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func registerMailHooks(t *testing.T, hooks extension.MailHooks) {
	t.Helper()
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
	registerMailHooks(t, extension.MailHooks{Deal: func(_ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		calls.Add(1)
		return extension.MailDeal{Outcome: "none", Reason: "Acme is on a credit hold"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook1@acme.example", "", "invoice"))
	wantDeals(t, e, "hook1@acme.example")
	if calls.Load() == 0 {
		t.Fatal("the hook was never called")
	}
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
	registerMailHooks(t, extension.MailHooks{Deal: func(_ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "linked", Deal: chosen.String(), Reason: "the subject names reporting"}, nil
	}})
	raw := mailFrom("alice@acme.example", "hook2@acme.example", "", "reporting question")
	env.sync(t, raw)
	wantDeals(t, e, "hook2@acme.example", chosen)
	mail := activityOf(t, e, "hook2@acme.example")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = '`+mail.String()+`'
		AND evidence::text LIKE '%deal-rules%' AND evidence::text LIKE '%the subject names reporting%'`); n != 1 {
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
	registerMailHooks(t, extension.MailHooks{Parties: func(_ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		p.Companies = []string{other.customer.String()}
		p.Reason = "the sender works for Elsewhere on this account"
		return p, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook3@acme.example", "", "hello"))
	wantDeals(t, e, "hook3@acme.example", elsewhere)

	registerMailHooks(t, extension.MailHooks{Parties: func(_ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		p.Companies = []string{ids.NewV7().String()}
		p.Reason = "a company that does not exist"
		return p, nil
	}})
	beta := w.at(t, "Beta")
	beta.employee(t, "bob@beta.example", beta.customer)
	only := beta.deal(t, "Beta rollout")
	env.sync(t, mailFrom("bob@beta.example", "hook4@beta.example", "", "hello"))
	wantDeals(t, e, "hook4@beta.example", only)
}
