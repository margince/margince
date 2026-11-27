// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEndingAPartnerProgramKeepsTheCompanyAndStoredTerms(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Program ends, company stays", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	tier := "tier2_20"
	partner, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: id, PartnerRole: "hosting", MarginTier: &tier})
	if err != nil {
		t.Fatal(err)
	}
	if partner.Version == nil {
		t.Fatal("partner has no version")
	}

	pipeline, stage, deal, commission := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pipeline (id, name, position) VALUES ($1, 'Partner history', 0)`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Open', 0, 'open', 10)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, name, pipeline_id, stage_id, source, captured_by) VALUES ($1, 'Partner history deal', $2, $3, 'manual', 'human:test')`, []any{deal, pipeline, stage}},
		{`INSERT INTO commission_entry (id, deal_id, partner_company_id, status, attribution_at_accrual, margin_tier_at_accrual, rate_bps, basis_amount_minor, currency, amount_minor, captured_by) VALUES ($1, $2, $3, 'paid', 'sourced', 'tier2_20', 2000, 100000, 'EUR', 20000, 'human:test')`, []any{commission, deal, id}},
	} {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, seed.sql, seed.args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	stale := *partner.Version - 1
	if err := e.store.EndPartnerProgram(ctx, id, &stale); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale end: %v", err)
	}
	if err := e.store.EndPartnerProgram(ctx, id, partner.Version); err != nil {
		t.Fatal(err)
	}
	assertPartnershipPaired(ctx, t, e, "ending the program")
	if err := e.store.EndPartnerProgram(ctx, id, partner.Version); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("second end: %v", err)
	}
	if _, err := e.store.GetPartner(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("live partner: %v", err)
	}
	kept, err := e.store.GetCompany(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if kept.PartnerProgramEnded == nil || !*kept.PartnerProgramEnded {
		t.Fatal("ended program history is absent from company read")
	}
	if kept.ArchivedAt != nil || liveTypesOf(ctx, t, e, id)[relationshipTypePartner] {
		t.Fatal("ending did not preserve a live company without the partner type")
	}
	var ended bool
	var storedTier string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL, margin_tier FROM partner WHERE company_id = $1`, id).Scan(&ended, &storedTier)
	}); err != nil {
		t.Fatal(err)
	}
	if !ended || storedTier != tier {
		t.Fatalf("ended=%v tier=%q", ended, storedTier)
	}

	var amount int64
	var status string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT amount_minor, status FROM commission_entry WHERE id = $1`, commission).Scan(&amount, &status)
	}); err != nil {
		t.Fatal(err)
	}
	if amount != 20000 || status != "paid" {
		t.Fatalf("commission changed: %d %s", amount, status)
	}
	restarted, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: id, PartnerRole: "hosting"})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if restarted.MarginTier == nil || string(*restarted.MarginTier) != tier || restarted.ActivatedAt != nil {
		t.Fatalf("restart terms or activation: %+v", restarted)
	}
	assertPartnershipPaired(ctx, t, e, "restarting the program")
}

func TestMergeRefusesALiveProgramIntoAnEndedSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	var companies []ids.CompanyID
	for _, name := range []string{"Ended survivor", "Live program source"} {
		company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		if _, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: id, PartnerRole: "hosting"}); err != nil {
			t.Fatal(err)
		}
		companies = append(companies, id)
	}
	if err := e.store.EndPartnerProgram(ctx, companies[0], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.MergeCompany(ctx, companies[1], companies[0], nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("merge: %v", err)
	}
	if _, err := e.store.GetPartner(ctx, companies[1]); err != nil {
		t.Fatalf("live source lost: %v", err)
	}
	assertPartnershipPaired(ctx, t, e, "refused merge")
}
