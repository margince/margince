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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/mailhooks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	mail := activityOf(t, e, "hook1@acme.example")
	wantFiling(t, e, mail, "none", "credit_hold")
	// No link was written, so no audit row names the unit. The system log does.
	wantHookLog(t, e, mail, "changed", "credit_hold", 1)
}

// A deal hook names a deal. The link lands and its audit evidence names the
// unit and the reason. A colleague then declines it: the hook cannot bring the
// deal back, and the next capture leaves the mark as it was.
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
	wantFiling(t, e, mail, "linked", "names_reporting")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = '`+mail.String()+`'
		AND evidence::text LIKE '%deal-rules%' AND evidence::text LIKE '%names_reporting%'`); n != 1 {
		t.Fatalf("%d audit rows name the unit and its reason, want 1", n)
	}
	wantHookLog(t, e, mail, "changed", "names_reporting", 0)

	if status, _ := decline(w.ctx, t, e, mail, dealBody(chosen)); status != http.StatusOK {
		t.Fatalf("the decline answered %d", status)
	}
	// The decline keeps the link and marks it unrelated.
	wantMark(t, e, "hook2@acme.example", chosen, "unrelated/human")
	// The next capture does not bring the hook's deal back.
	env.sync(t, raw)
	wantMark(t, e, "hook2@acme.example", chosen, "unrelated/human")
	if slices.Contains(dealsOf(t, e, "hook2@acme.example"), operations) {
		t.Fatal("the hook's declined deal moved the mail onto the other one")
	}
}

// The deal's timeline read says which unit filed the mail and why, until a
// colleague's decline takes the filing back. Core's own filing names core.
func TestTheActivityReadNamesWhoFiledAMailOnItsDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	env.sync(t, mailFrom("alice@acme.example", "filer1@acme.example", "", "status"))
	wantDeals(t, e, "filer1@acme.example", only)
	if got := filedBy(t, e, w, activityOf(t, e, "filer1@acme.example"), only); got != "core//" {
		t.Errorf("core's filing reads as %q, want core", got)
	}
	chosen := w.deal(t, "Acme reporting")
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, _ extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "linked", Deal: chosen.String(), Reason: "names_reporting"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "filer2@acme.example", "", "reporting question"))
	mail := activityOf(t, e, "filer2@acme.example")
	if got := filedBy(t, e, w, mail, chosen); got != "deal-rules/deal/names_reporting" {
		t.Errorf("the unit's filing reads as %q, want deal-rules/deal/names_reporting", got)
	}
	// The classifier's later mark on the link does not take the filing over.
	system := newRepairer(e, w).system
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		_, err := activities.MarkDealLinkTx(system, tx, ids.From[ids.ActivityKind](mail), chosen, "uncertain", nil)
		return err
	}); err != nil {
		t.Fatalf("the classifier's mark: %v", err)
	}
	wantMark(t, e, "filer2@acme.example", chosen, "uncertain/classifier")
	if got := filedBy(t, e, w, mail, chosen); got != "deal-rules/deal/names_reporting" {
		t.Errorf("after the classifier's mark the filing reads as %q, want the unit still", got)
	}
	if status, _ := decline(w.ctx, t, e, mail, dealBody(chosen)); status != http.StatusOK {
		t.Fatalf("the decline answered %d", status)
	}
	if got := filedBy(t, e, w, mail, chosen); got != "" {
		t.Errorf("after a colleague's decline the filing reads as %q, want none", got)
	}
}

// filedBy reads the mail as the deal world's colleague and spells the filer of
// its link to deal as actor/hook/reason, or "" when it names none.
func filedBy(t *testing.T, e *integration.SearchEnv, w dealWorld, mail, deal ids.UUID) string {
	t.Helper()
	got, err := activities.NewStore(e.DB()).GetActivity(w.ctx, ids.From[ids.ActivityKind](mail), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the mail: %v", err)
	}
	for _, link := range deref(got.Links) {
		if link.EntityType != "deal" || ids.UUID(link.EntityId) != deal || link.FiledBy == nil {
			continue
		}
		hook, reason := "", ""
		if link.FiledBy.Hook != nil {
			hook = string(*link.FiledBy.Hook)
		}
		if link.FiledBy.Reason != nil {
			reason = *link.FiledBy.Reason
		}
		return link.FiledBy.Actor + "/" + hook + "/" + reason
	}
	return ""
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

// A company has three open deals, and core's own reads name only the first two.
// A hook may name any open deal of the company, the one with the highest id too.
func TestADealHookCanNameAnyOpenDealOfTheCompanyNotOnlyTheFirstTwo(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	open := []ids.UUID{w.deal(t, "Acme rollout"), w.deal(t, "Acme renewal"), w.deal(t, "Acme expansion")}
	last := slices.MaxFunc(open, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, _ extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "linked", Deal: last.String(), Reason: "purchase_order"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook9@acme.example", "", "purchase order"))
	if got := dealsOf(t, e, "hook9@acme.example"); len(got) != 1 || got[0] != last {
		t.Fatalf("the mail is filed on %v, want the deal %v", got, last)
	}
	wantFiling(t, e, activityOf(t, e, "hook9@acme.example"), "linked", "purchase_order")
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

// A hook that fails decides nothing. Core does not fall back to its own answer.
// The mail stays unfiled and the trace says which unit failed. The next capture
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
	wantFiling(t, e, activityOf(t, e, "hook7@acme.example"), "failed", "")
	wantHookLog(t, e, activityOf(t, e, "hook7@acme.example"), "failed", "", 1)

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
		return extension.MailDeal{Outcome: "none", Reason: "alice@acme.example wrote: invoice"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "hook8@acme.example", "", "invoice"))
	wantDeals(t, e, "hook8@acme.example", only)
	mail := activityOf(t, e, "hook8@acme.example")
	wantFiling(t, e, mail, "ignored", "alice@acme.example wrote: invoice")
	// Core's own link was written, and its audit row names no refused answer.
	// The lasting row keeps a marker, never the hook's text, which may be mail.
	wantHookLog(t, e, mail, "ignored", "undeclared", 1)
	if n := countRows(t, e, `SELECT count(*) FROM system_log WHERE action = 'extension_mail_hook'
		AND detail::text LIKE '%alice@acme.example%'`); n != 0 {
		t.Fatalf("%d system log rows keep the hook's undeclared reason", n)
	}
}

// A repair decides a mail once per mailbox that imported it, in one
// transaction. A hook that fails for both leaves one lasting row, not two.
func TestARepairAcrossTwoMailboxesLogsAHookAnswerOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	colleague := secondSentMailbox(t, e, e.Rep3)
	raw := mailFrom("alice@acme.example", "hook11@acme.example", "", "invoice", secondSeatAddress)
	env.sync(t, raw)
	colleague(t, nil, nil, raw)
	wantDeals(t, e, "hook11@acme.example", only)
	mail := activityOf(t, e, "hook11@acme.example")
	if n := countRows(t, e, `SELECT count(DISTINCT user_id) FROM capture_import WHERE activity_id = '`+mail.String()+`'`); n != 2 {
		t.Fatalf("the mail came from %d mailboxes, want 2 for this test to hold anything", n)
	}

	var calls atomic.Int32
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		calls.Add(1)
		return d, errors.New("the ledger is down")
	}})
	r := newRepairer(e, w)
	r.repair.Hooks = mailhooks.New(compose.ComposedExtensions, e.Pool)
	r.run(t)
	wantDeals(t, e, "hook11@acme.example", only)
	if calls.Load() < 2 {
		t.Fatalf("the hook ran %d times, want once per mailbox", calls.Load())
	}
	wantHookLog(t, e, mail, "failed", "", 1)
}

// A hook now answers `none` on a mail the rule filed, and the repair takes the
// link back. The unlink's audit row names the unit, and so does the trace.
func TestARepairThatAHookMakesTakeALinkBackNamesTheUnit(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	env.sync(t, mailFrom("alice@acme.example", "hook10@acme.example", "", "invoice"))
	wantDeals(t, e, "hook10@acme.example", only)

	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "none", Reason: "credit_hold"}, nil
	}})
	r := newRepairer(e, w)
	r.repair.Hooks = mailhooks.New(compose.ComposedExtensions, e.Pool)
	r.run(t)
	wantDeals(t, e, "hook10@acme.example")
	mail := activityOf(t, e, "hook10@acme.example")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+mail.String()+`'
		AND evidence::text LIKE '%deal-rules%' AND evidence::text LIKE '%credit_hold%'`); n != 1 {
		t.Fatalf("%d audit rows name the unit that took the link back, want 1", n)
	}
	wantFiling(t, e, mail, "retracted", "credit_hold")
	wantHookLog(t, e, mail, "changed", "credit_hold", 0)
}

// A parties hook cannot name a company the mailbox owner may not see. The
// answer is ignored and the trace says so. No deal of that company is reached.
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
			`UPDATE company SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden.customer, e.Rep3)
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
	wantFiling(t, e, activityOf(t, e, "hook9@acme.example"), "ignored", "other_account")
}

// wantHookLog asserts how many system log rows record a deal-rules answer of
// this result and reason on the mail.
func wantHookLog(t *testing.T, e *integration.SearchEnv, mail ids.UUID, result, reason string, want int) {
	t.Helper()
	if n := countRows(t, e, `SELECT count(*) FROM system_log WHERE action = 'extension_mail_hook'
		AND detail->>'activity_id' = '`+mail.String()+`' AND detail->>'unit' = 'deal-rules'
		AND detail->>'result' = '`+result+`' AND coalesce(detail->>'reason', '') = '`+reason+`'`); n != want {
		t.Fatalf("%d system log rows record deal-rules %s the mail (reason %q), want %d", n, result, reason, want)
	}
}

// wantFiling asserts what the mail's capture trace says the deal-rules unit did.
func wantFiling(t *testing.T, e *integration.SearchEnv, mail ids.UUID, result, reason string) {
	t.Helper()
	if n := countRows(t, e, `SELECT count(*) FROM capture_trace WHERE activity_id = '`+mail.String()+`'
		AND filing->>'actor' = 'deal-rules' AND filing->>'result' = '`+result+`'
		AND coalesce(filing->>'reason', '') = '`+reason+`'`); n < 1 {
		t.Fatalf("the trace does not say deal-rules %s the mail (reason %q)", result, reason)
	}
}
