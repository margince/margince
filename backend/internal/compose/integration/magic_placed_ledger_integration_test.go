// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal suggestion, an invoice and a payment have no page of their own either.
// Done for you places each on its company, behind the child's own read rule.

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// grantFinanceObject is the object grant every finance read asks.
const grantFinanceObject = "finance"

// ledgerSync records whether the fixture's accounting source has synced, since
// one installation connects one source and its ledger lands once.
type ledgerSync struct{ done bool }

// ledgerWriters write a suggestion, an invoice and a payment through their own
// writers, as the system lanes that write them.
func ledgerWriters(f placedFixture) map[string]func(t *testing.T) (key string, parent ids.UUID) {
	return map[string]func(t *testing.T) (string, ids.UUID){
		"deal_suggestion": func(t *testing.T) (string, ids.UUID) {
			company, _ := f.writeSuggestion(t)
			return "magic.action.create_deal_suggestion", company
		},
		"finance_invoice": func(t *testing.T) (string, ids.UUID) {
			f.syncLedger(t)
			return "magic.action.create_invoice", f.company
		},
		"finance_payment": func(t *testing.T) (string, ids.UUID) {
			f.syncLedger(t)
			return "magic.action.create_payment", f.company
		},
	}
}

// writeSuggestion has the scout suggest a deal from one held meeting, on a
// company of Rep1's with no deal yet. It answers the company and the meeting.
func (f placedFixture) writeSuggestion(t *testing.T) (company, meeting ids.UUID) {
	t.Helper()
	e := f.e
	company = e.SeedCompany(t, "Suggested Co", &e.Rep1)
	buyer := e.SeedContact(t, "Suggested Buyer", &e.Rep1)
	held, subject, at := "held", "Scoping workshop", clocktest.Now(t).Add(-72*time.Hour).Truncate(time.Microsecond)
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	})
	mustWrite(t, err)
	meeting = ids.UUID(logged.Id)
	scout := principal.SystemActing(e.Admin(), "system:deal-scout")
	mustWrite(t, database.WithTx(scout, e.Pool, func(tx pgx.Tx) error {
		raised, err := deals.RecordSuggestionTx(scout, tx, deals.SuggestionDraft{
			CompanyID: company, NameHint: deals.HintMeetingHeld, Confidence: 0.6,
			Evidence: []deals.SuggestionEvidence{{Kind: deals.EvidenceMeeting, ActivityID: &meeting, OccurredAt: at}},
		})
		if err == nil && !raised {
			err = errors.New("the scout raised no suggestion")
		}
		return err
	}))
	return company, meeting
}

// syncLedger links the source's first customer to the fixture's company and
// has the sweep mirror its invoice and payment.
func (f placedFixture) syncLedger(t *testing.T) {
	t.Helper()
	if f.ledger.done {
		return
	}
	f.ledger.done = true
	store := financeStoreFor(f.e, testBooksSource{paid: true})
	admin := connectAndSync(t, f.e, store)
	_, err := store.LinkCustomer(admin, "CUST-A", companyIDOf(f.company))
	mustWrite(t, err)
	_, err = store.SyncConnection(principal.SystemActing(admin, "finance-sync"), testBooksSource{paid: true})
	mustWrite(t, err)
}

// readerWithout is Rep1 holding the placed grants, less one.
func (f placedFixture) readerWithout(object string) context.Context {
	perms := withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, object)
	return f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms)
}

// An invoice and a payment are read with the finance grant. A seat without it
// loses both lines and keeps the suggestion.
func TestDoneForYouHidesTheLedgerFromASeatWithoutFinance(t *testing.T) {
	f := newPlacedFixture(t)
	writers := ledgerWriters(f)
	writers["finance_invoice"](t)
	writers["deal_suggestion"](t)
	home, worklist := placedLines(f.readerWithout(grantFinanceObject), t, f.e)
	for _, key := range []string{"magic.action.create_invoice", "magic.action.create_payment"} {
		if _, shown := home[key]; shown {
			t.Errorf("without finance, Home shows %s", key)
		}
		if _, shown := worklist[key]; shown {
			t.Errorf("without finance, the Worklist shows %s", key)
		}
	}
	if _, kept := home["magic.action.create_deal_suggestion"]; !kept {
		t.Fatalf("a missing finance grant took the suggestion with it: %v", home)
	}
}

// A suggestion is read only by a seat that may read every piece of its
// evidence. Without the activity grant its meeting is unreadable, so the line
// is hidden although the company and the deal grant are held.
func TestDoneForYouHidesASuggestionWhoseEvidenceTheReaderCannotRead(t *testing.T) {
	f := newPlacedFixture(t)
	company, meeting := f.writeSuggestion(t)
	full, _ := placedLines(f.readerWithout(""), t, f.e)
	if full["magic.action.create_deal_suggestion"] != company {
		t.Fatalf("a reader of the evidence is not shown the suggestion: %v", full)
	}
	for _, object := range []string{objActivity, objDeal} {
		home, worklist := placedLines(f.readerWithout(object), t, f.e)
		if _, shown := home["magic.action.create_deal_suggestion"]; shown {
			t.Errorf("without %s, Home shows the suggestion", object)
		}
		if _, shown := worklist["magic.action.create_deal_suggestion"]; shown {
			t.Errorf("without %s, the Worklist shows the suggestion", object)
		}
	}
	// Every grant held, but the meeting is now its participants' alone.
	f.e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, meeting)
	home, worklist := placedLines(f.readerWithout(""), t, f.e)
	if _, shown := home["magic.action.create_deal_suggestion"]; shown {
		t.Error("Home shows a suggestion resting on a meeting the reader cannot read")
	}
	if _, shown := worklist["magic.action.create_deal_suggestion"]; shown {
		t.Error("the Worklist shows a suggestion resting on a meeting the reader cannot read")
	}
}

// ledgerReads are the reads of one company's invoices and payments, each
// answering whether it showed the reader anything of the fixture's ledger.
func (f placedFixture) ledgerReads(t *testing.T) map[string]func(ctx context.Context) bool {
	t.Helper()
	store := financeStoreFor(f.e, testBooksSource{paid: true})
	company := companyIDOf(f.company)
	invoice := uuidOf(t, f.e.WsScalar(t, `SELECT id::text FROM finance_invoice WHERE company_id = $1`, f.company))
	payment := uuidOf(t, f.e.WsScalar(t, `SELECT id::text FROM finance_payment WHERE company_id = $1`, f.company))
	shown := func(t *testing.T, err error) bool {
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
			t.Fatalf("a ledger read failed: %v", err)
		}
		return !errors.Is(err, apperrors.ErrNotFound)
	}
	return map[string]func(ctx context.Context) bool{
		"customer payments": func(ctx context.Context) bool {
			out, err := store.CustomerPayments(ctx, nil, nil)
			mustWrite(t, err)
			for _, row := range out.Data {
				if ids.UUID(row.CompanyId) == f.company {
					return true
				}
			}
			return false
		},
		"the finance summary": func(ctx context.Context) bool {
			_, err := store.SummaryFor(ctx, company)
			return shown(t, err)
		},
		"the invoice reference": func(ctx context.Context) bool {
			_, _, err := store.InvoiceReference(ctx, invoice)
			return shown(t, err)
		},
		// The test source serves no document, so the owner's answer is that
		// refusal and a hidden ledger's is the 404 a missing invoice gets.
		"the invoice document": func(ctx context.Context) bool {
			_, err := store.InvoiceDocumentFor(ctx, invoice)
			return shown(t, err)
		},
		"the invoice in a merge review": func(ctx context.Context) bool {
			_, err := store.MergeReference(ctx, "invoice", invoice)
			return shown(t, err)
		},
		"the payment in a merge review": func(ctx context.Context) bool {
			_, err := store.MergeReference(ctx, "payment", payment)
			return shown(t, err)
		},
		// The attention feed's handled lane reads the Worklist's reader.
		homeLedger: func(ctx context.Context) bool {
			home, worklist := placedLines(ctx, t, f.e)
			_, onHome := home["magic.action.create_invoice"]
			_, onWorklist := worklist["magic.action.create_invoice"]
			return onHome || onWorklist
		},
	}
}

// homeLedger names the read that shows only records the reader works on. An
// admin reads every ledger but works on none of these records.
const homeLedger = "Home and the Worklist"

// An invoice can carry a salary. A seat with the finance and
// company grants reads a company's ledger only in three cases. It owns the
// company, holds a record grant on it, or has every row in scope. A teammate of the owner and a seat
// on another team read none of it, through any read; each is refused with 404.
func TestOnlyTheOwnerReadsACompanysLedger(t *testing.T) {
	f := newPlacedFixture(t)
	f.syncLedger(t)
	perms := withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})
	stranger := f.e.As(f.e.Rep3, []ids.UUID{f.e.Team2}, perms)
	if _, err := f.e.Contacts.GetCompany(stranger, companyIDOf(f.company), storekit.LiveOnly); err != nil {
		t.Fatalf("the non-owner cannot read the company at all, so the case proves nothing: %v", err)
	}
	reads := f.ledgerReads(t)
	check := func(name string, ctx context.Context, sees bool) {
		t.Helper()
		for read, shows := range reads {
			if name == "an admin" && read == homeLedger {
				continue
			}
			if got := shows(ctx); got != sees {
				t.Errorf("%s is shown %s: %v, want %v", name, read, got, sees)
			}
		}
	}
	check("the owner", f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms), true)
	check("a teammate of the owner", f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, perms), false)
	check("a seat on another team", stranger, false)
	check("an admin", f.e.As(f.e.AdminUser, nil, withPlacedGrants(AdminPerms, principal.ObjectGrant{Read: true})), true)
	f.e.ShareForWriting(t, "company", f.company, "user", f.e.Rep3)
	check("a seat the company is shared with", stranger, true)
}
