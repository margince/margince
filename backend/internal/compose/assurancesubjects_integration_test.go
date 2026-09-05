// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The subject seam against a real database.
//
// The census here is the point: assurance.Subject once had fourteen fields and
// the seam populated nine, so a scheduled pass would have raised "no next
// step" and "no economic buyer" against every deal in the installation — the
// unpopulated zero value is exactly the shape those rules fire on. The dressed
// fixture below makes every field non-zero, and the reflection walk fails the
// moment a new rule input exists that this fixture (and therefore the seam)
// does not feed.

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dressedDeal seeds one open deal carrying a value in every Subject field: a
// commit-category deal with a provisional close date, an inbound email, a
// same-currency sent offer and active contract, two close-date pushes plus one
// pull-in that must not count, an open task, and an economic buyer.
func dressedDeal(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	pipeline, open, _ := integration.DealFixture(t, e)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	deal := integration.SeedIDRow(t, owner, `
		INSERT INTO deal (id, pipeline_id, stage_id, name, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date, close_date_provisional,
		                  forecast_category)
		VALUES ($1, $2, $3, 'Dressed Subject', $4, 'open', 'manual', 'test',
		        4200000, 'EUR', now()::date + 30, true, 'commit')`,
		pipeline, open, e.Rep1)

	inbound := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'they wrote', now() - interval '2 days', 'inbound', 'manual', 'test')`)
	integration.LinkActivity(t, owner, inbound, "deal", deal)

	task := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, is_done, source, captured_by)
		VALUES ($1, 'task', 'Send the revised offer', now(), false, 'manual', 'test')`)
	integration.LinkActivity(t, owner, task, "deal", deal)

	if _, err := owner.Exec(ctx, `
		INSERT INTO offer (deal_id, offer_number, status, currency, gross_minor, source, captured_by)
		VALUES ($1, 'O-1', 'sent', 'EUR', 4300000, 'manual', 'test')`, deal); err != nil {
		t.Fatalf("seeding the sent offer: %v", err)
	}
	// A second, newer offer in another currency must NOT become the total: a
	// USD minor count next to an EUR deal amount is not a discrepancy, it is an
	// exchange rate.
	if _, err := owner.Exec(ctx, `
		INSERT INTO offer (deal_id, offer_number, status, currency, gross_minor, source, captured_by, updated_at)
		VALUES ($1, 'O-2', 'sent', 'USD', 9900000, 'manual', 'test', now() + interval '1 minute')`,
		deal); err != nil {
		t.Fatalf("seeding the foreign-currency offer: %v", err)
	}

	org := integration.SeedIDRow(t, owner, `
		INSERT INTO organization (id, display_name, source, captured_by)
		VALUES ($1, 'Buyer GmbH', 'manual', 'test')`)
	if _, err := owner.Exec(ctx, `
		INSERT INTO contract (organization_id, deal_id, title, value_minor, currency,
		                      status, signed_on, source, captured_by)
		VALUES ($1, $2, 'MSA', 4100000, 'EUR', 'active', now()::date, 'manual', 'test')`,
		org, deal); err != nil {
		t.Fatalf("seeding the signed contract: %v", err)
	}

	person := integration.SeedIDRow(t, owner, `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Signer', 'manual', 'test')`)
	if _, err := owner.Exec(ctx, `
		INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'test')`,
		person, deal); err != nil {
		t.Fatalf("seeding the economic buyer: %v", err)
	}

	// Two pushes and one pull-in. Only moves to a LATER date count: pulling a
	// date closer is the opposite of the pattern the rule pages about.
	for _, dates := range [][2]string{
		{"2026-09-01", "2026-09-15"},
		{"2026-09-15", "2026-10-01"},
		{"2026-10-01", "2026-09-20"},
	} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type,
			                       entity_id, before, after)
			VALUES ('human', 'test', 'update', 'deal', $1,
			        jsonb_build_object('expected_close_date', $2::text),
			        jsonb_build_object('expected_close_date', $3::text))`,
			deal, dates[0], dates[1]); err != nil {
			t.Fatalf("seeding a close-date move: %v", err)
		}
	}
	return deal
}

func subjectsFor(t *testing.T, e *integration.Env) []assurance.Subject {
	t.Helper()
	var out []assurance.Subject
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = AssuranceSubjects(context.Background(), tx)
		return err
	}); err != nil {
		t.Fatalf("reading the subjects: %v", err)
	}
	return out
}

func TestEveryRuleInputHasAnAssembledSource(t *testing.T) {
	e := integration.Setup(t)
	deal := dressedDeal(t, e)

	var subject *assurance.Subject
	for _, s := range subjectsFor(t, e) {
		if s.DealID == deal.String() {
			found := s
			subject = &found
		}
	}
	if subject == nil {
		t.Fatal("the dressed deal is not among the subjects")
	}

	// The census: a Subject field the dressed fixture leaves at its zero value
	// is a rule input the seam does not assemble — the exact hole that would
	// have raised false findings against every deal in the installation.
	v := reflect.ValueOf(*subject)
	for i := range v.NumField() {
		if v.Field(i).IsZero() {
			t.Errorf("Subject.%s is the zero value on a fixture dressed to fill "+
				"every field: either the seam does not assemble it or this fixture "+
				"must grow with the new rule input", v.Type().Field(i).Name)
		}
	}

	if got := *subject.OfferTotalMinor; got != 4300000 {
		t.Errorf("offer total = %d, want the newest SAME-CURRENCY sent offer (4300000); "+
			"a foreign-currency total would report an exchange rate as a discrepancy", got)
	}
	if got := *subject.ContractTotalMinor; got != 4100000 {
		t.Errorf("contract total = %d, want 4100000", got)
	}
	if subject.CloseDatePushes != 2 {
		t.Errorf("close-date pushes = %d, want 2: two moves later count, the pull-in does not",
			subject.CloseDatePushes)
	}
	if subject.NextStep != "Send the revised offer" {
		t.Errorf("next step = %q, want the open task's subject", subject.NextStep)
	}
}

// The mailbox is graded by connector health, not mail volume: a quiet mailbox
// on a healthy connector is checked, and no connector at all is not_connected
// rather than a repair that nobody can perform.
func TestMailCoverageGradesTheConnectorNotTheTraffic(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	now := time.Now().UTC()

	read := func() assurance.SourceCoverage {
		t.Helper()
		var out assurance.SourceCoverage
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			for _, c := range AssuranceCoverage(context.Background(), tx, now) {
				if c.Source == "mail" {
					out = c
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("reading coverage: %v", err)
		}
		return out
	}

	if got := read(); got.State != assurance.CoverageNotConnected {
		t.Fatalf("no mail connection reads %q, want not_connected", got.State)
	}

	conn := integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'gmail', $2, 'connected')`, e.Rep1)
	if got := read(); got.State != assurance.CoverageUnavailable {
		t.Fatalf("connected but never synced reads %q, want unavailable", got.State)
	}

	if _, err := owner.Exec(ctx, `
		INSERT INTO capture_sync_state (connection_id, last_success_at)
		VALUES ($1, now() - interval '3 hours')`, conn); err != nil {
		t.Fatalf("seeding the checkpoint: %v", err)
	}
	got := read()
	if got.State != assurance.CoverageChecked {
		t.Fatalf("healthy checkpoint over a mailbox with NO mail reads %q, want checked "+
			"— a quiet week is not a broken connector", got.State)
	}
	if got.CheckedThrough == nil {
		t.Fatal("a checked source carries the instant it was read through")
	}

	if _, err := owner.Exec(ctx, `
		UPDATE capture_sync_state SET last_success_at = now() - interval '5 days'
		WHERE connection_id = $1`, conn); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.State != assurance.CoverageStale {
		t.Fatalf("a checkpoint five days behind reads %q, want stale", got.State)
	}

	if _, err := owner.Exec(ctx, `
		UPDATE capture_connection SET status = 'reauth_required' WHERE id = $1`, conn); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.State != assurance.CoveragePermissionLimited {
		t.Fatalf("a connector needing re-auth reads %q, want permission_limited", got.State)
	}
}

// An empty native offers table is checked-and-found-nothing, not a broken
// source: it lives in the same database as the run itself.
func TestAnEmptyOfferTableIsCheckedNotUnavailable(t *testing.T) {
	e := integration.Setup(t)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, c := range AssuranceCoverage(context.Background(), tx, time.Now().UTC()) {
			if c.Source == "offers" && c.State != assurance.CoverageChecked {
				t.Errorf("an empty offer table reads %q, want checked", c.State)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
