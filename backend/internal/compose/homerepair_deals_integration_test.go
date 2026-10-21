// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReviewedSuggestionRepairPreservesCurrentEvidenceAndRetiresExpiredQualification(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Example buyer", nil)
	e.buyerPair(t, company, e.daysAgo(6))
	e.pass(t)
	reader := e.signalReader()
	suggestion := e.onlySuggestion(reader, t, company)
	later := e.now.Add(dealScoutWindow + 24*time.Hour)
	provenance := deals.SuggestionRepairProvenance{RuleVersion: homeRepairVersion, RunID: ids.NewV7()}
	if err := e.DB().Tx(reader, func(tx pgx.Tx) error {
		current, err := deals.PreviewSuggestionRepairTx(reader, tx, suggestion.ID, e.now, dealSuggestionEffects{}.BuyerStillVerified)
		if err != nil {
			return err
		}
		if current.Supersede || current.Reason != "current_work" || current.Version < 1 {
			t.Fatalf("current suggestion=%+v", current)
		}
		expired, err := deals.PreviewSuggestionRepairTx(reader, tx, suggestion.ID, later, dealSuggestionEffects{}.BuyerStillVerified)
		if err != nil {
			return err
		}
		if !expired.Supersede || expired.Reason != "buyer_no_longer_verified" {
			t.Fatalf("expired qualification=%+v", expired)
		}
		applied, err := deals.SupersedeReviewedSuggestionTx(reader, tx, expired, provenance, later, dealSuggestionEffects{}.BuyerStillVerified)
		if err == nil && !applied {
			t.Fatal("reviewed retirement was not applied")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state := e.WsScalar(t, "SELECT state FROM deal_suggestion WHERE id=$1", suggestion.ID)
	if state != deals.SuggestionSuperseded || e.WsCount(t, "SELECT count(*) FROM deal") != 0 {
		t.Fatalf("repair state=%s; retirement must not create a deal", state)
	}
	e.now = later
	e.pass(t)
	if len(e.suggestions(reader, t, company)) != 0 {
		t.Fatal("retired evidence was offered again")
	}
	if count := e.WsCount(t, "SELECT count(*) FROM audit_log WHERE entity_type='deal_suggestion' AND entity_id=$1 AND after->>'state'='superseded' AND before->>'state'='open' AND evidence->>'reason'='buyer_no_longer_verified' AND evidence->>'rule_version'=$2 AND evidence->>'run_id'=$3", suggestion.ID, provenance.RuleVersion, provenance.RunID.String()); count != 1 {
		t.Fatalf("retirement audit count=%d", count)
	}
}

func TestReviewedSuggestionRepairPreservesALaterHumanDismissal(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Example buyer", nil)
	e.buyerPair(t, company, e.daysAgo(6))
	e.pass(t)
	reader := e.signalReader()
	suggestion := e.onlySuggestion(reader, t, company)
	later := e.now.Add(dealScoutWindow + 24*time.Hour)
	provenance := deals.SuggestionRepairProvenance{RuleVersion: homeRepairVersion, RunID: ids.NewV7()}
	var reviewed deals.SuggestionRepair
	if err := e.DB().Tx(reader, func(tx pgx.Tx) error {
		page, err := deals.SuggestionRepairPageTx(reader, tx, ids.Nil, 25, later, dealSuggestionEffects{}.BuyerStillVerified)
		if err != nil {
			return err
		}
		if len(page) != 1 || !page[0].Supersede {
			t.Fatalf("review page=%+v", page)
		}
		reviewed = page[0]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.decider().DismissSuggestion(reader, suggestion.ID); err != nil {
		t.Fatal(err)
	}
	err := e.DB().Tx(reader, func(tx pgx.Tx) error {
		applied, err := deals.SupersedeReviewedSuggestionTx(reader, tx, reviewed, provenance, later, dealSuggestionEffects{}.BuyerStillVerified)
		if applied {
			t.Fatal("repair overwrote the human dismissal")
		}
		return err
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("changed review error=%v", err)
	}
	if state := e.WsScalar(t, "SELECT state FROM deal_suggestion WHERE id=$1", suggestion.ID); state != deals.SuggestionDismissed {
		t.Fatalf("human decision became %s", state)
	}
}
