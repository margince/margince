// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// One edit is one version. The row's version trigger fires on every UPDATE.
// An edit that follows its write with a second statement on the same row
// leaves a hole in the sequence that a client cannot predict.

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedManualLead(t *testing.T) (context.Context, *Store, crmcontracts.Lead, ids.LeadID) {
	t.Helper()
	ctx, store := newLeadScoreEnv(t)
	created, _, err := store.CreateLead(ctx, CreateLeadInput{
		FullName: new("Jonas Petersen"), Email: new("jonas@nordwind.example"),
		Title: new("VP Sales"), Source: "manual", Status: "new",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, created, ids.From[ids.LeadKind](ids.UUID(created.Id))
}

// The rescore an origin change owes lands in the series with the number the lead
// now shows. The edit wrote that number in the same UPDATE.
func TestARescoreOnAnOriginChangeIsInTheSeriesAndTheAuditDiff(t *testing.T) {
	ctx, store, _, id := seedManualLead(t)
	updated, err := store.UpdateLead(ctx, id, UpdateLeadInput{Origin: new("referral")})
	if err != nil {
		t.Fatal(err)
	}
	explained, err := store.ExplainLeadScore(ctx, id, ExplainLeadScoreInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if explained.Current == nil || explained.Current.Score != updated.Score {
		t.Fatalf("newest series entry %+v does not show the lead's score %d", explained.Current, updated.Score)
	}
	var after string
	if err := store.db.Pool().QueryRow(ctx,
		`SELECT after::text FROM audit_log WHERE entity_type = 'lead' AND entity_id = $1
		  ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, `"origin"`) || strings.Contains(after, `"score"`) {
		t.Fatalf("the edit's audit row should carry the origin and not the score derived from it, got %s", after)
	}
}

// Under a Commercial Judgement override the rescore moves the machine value
// and leaves the human's number alone, still in one version.
func TestAnOriginChangeUnderAnOverrideMovesOnlyTheMachineScore(t *testing.T) {
	ctx, store, _, id := seedManualLead(t)
	human, reason := 88, "strategic account"
	set, err := store.UpdateLead(ctx, id, UpdateLeadInput{Score: &human, ScoreOverrideReason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateLead(ctx, id, UpdateLeadInput{Origin: new("referral"), IfVersion: set.Version})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Score != human {
		t.Fatalf("score = %d, want the human's %d", updated.Score, human)
	}
	if updated.ScoreComputed == nil || *updated.ScoreComputed == *set.ScoreComputed {
		t.Fatalf("machine score should follow the origin: before %v, after %v", set.ScoreComputed, updated.ScoreComputed)
	}
	if got, want := *updated.Version, *set.Version+1; got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
}

// An origin whose weight equals the old one moves no score, writes no series
// entry, and still raises the version once.
func TestAnOriginChangeThatLeavesTheScoreAlonePutsNothingInTheSeries(t *testing.T) {
	ctx, store, _, id := seedManualLead(t)
	low, err := store.UpdateLead(ctx, id, UpdateLeadInput{Origin: new("event")})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.ExplainLeadScore(ctx, id, ExplainLeadScoreInput{History: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateLead(ctx, id, UpdateLeadInput{Origin: new("other"), IfVersion: low.Version})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Score != low.Score {
		t.Fatalf("premise: two neutral origins must score alike, got %d then %d", low.Score, updated.Score)
	}
	after, err := store.ExplainLeadScore(ctx, id, ExplainLeadScoreInput{History: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(*after.History) != len(*before.History) {
		t.Fatalf("series grew from %d to %d though the score did not move", len(*before.History), len(*after.History))
	}
	if got, want := *updated.Version, *low.Version+1; got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
}

// Every field a company edit can move is one version. The follow-up writes an
// edit owes do not change that: provenance, the geocode after an address move,
// the anchor name after a rename.
func TestEachCompanyEditRaisesItsVersionByOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	edits := map[string]UpdateCompanyInput{
		"display_name": {DisplayName: new("Renamed Co")},
		"legal_name":   {LegalName: new("Renamed Co GmbH")},
		"description":  {Description: new("Sells glass.")},
		"industry":     {Industry: new("construction")},
		"address": {Address: &crmcontracts.Address{
			Line1: new("1 Hafenstrasse"), City: new("Hamburg"), Country: new("DE"),
		}},
	}
	for field, in := range edits {
		t.Run(field, func(t *testing.T) {
			created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Glazed Frog " + field, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			in.IfVersion = created.Version
			updated, err := e.store.UpdateCompany(ctx, ids.From[ids.CompanyKind](ids.UUID(created.Id)), in)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := *updated.Version, *created.Version+1; got != want {
				t.Fatalf("version after editing %s = %d, want %d", field, got, want)
			}
		})
	}
}

func TestEachContactEditRaisesItsVersionByOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	edits := map[string]UpdateContactInput{
		"full_name": {FullName: new("Renamed Pat")},
		"title":     {Title: new("CTO")},
		"address":   {Address: &crmcontracts.Address{City: new("Hamburg"), Country: new("DE")}},
	}
	for field, in := range edits {
		t.Run(field, func(t *testing.T) {
			created, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Pat " + field, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			in.IfVersion = created.Version
			updated, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(created.Id)), in)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := *updated.Version, *created.Version+1; got != want {
				t.Fatalf("version after editing %s = %d, want %d", field, got, want)
			}
		})
	}
}

func TestAHumanRenameStampsTheNameAsHumanChosen(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Glazed Frog GmbH", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(created.Id))
	renamed := "Glazed Toad GmbH"
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{DisplayName: &renamed, IfVersion: created.Version}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := companyColumn(ctx, t, e, id, "name_source"); got != nameSourceHuman {
		t.Fatalf("a human rename stamps name_source %q, got %q", nameSourceHuman, got)
	}
}

func TestAnOriginChangeThatMovesTheScoreRaisesItsVersionByOne(t *testing.T) {
	ctx, store, created, id := seedManualLead(t)
	updated, err := store.UpdateLead(ctx, id, UpdateLeadInput{Origin: new("referral")})
	if err != nil {
		t.Fatalf("change origin: %v", err)
	}
	if updated.Score == created.Score {
		t.Fatalf("premise: the origin change must move the score, both %d", created.Score)
	}
	if got, want := *updated.Version, *created.Version+1; got != want {
		t.Fatalf("version after an origin change that rescored = %d, want %d", got, want)
	}
}

// Withdrawing an override resumes the machine score. When that score has moved
// since the override took hold, the new number rides the clear's own UPDATE.
func TestClearingAnOverrideRaisesTheVersionByOneWhenTheMachineScoreMoved(t *testing.T) {
	ctx, store, _, id := seedManualLead(t)
	human, reason := 88, "strategic account"
	set, err := store.UpdateLead(ctx, id, UpdateLeadInput{Score: &human, ScoreOverrideReason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Pool().Exec(ctx, `UPDATE lead SET score_computed = score_computed + 1 WHERE id = $1`, id); err != nil {
		t.Fatalf("simulating a machine score that moved under the override: %v", err)
	}
	stale, err := store.GetLead(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := store.UpdateLead(ctx, id, UpdateLeadInput{ClearScoreOverride: true, IfVersion: stale.Version})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Score == *stale.ScoreComputed {
		t.Fatalf("premise: the resumed score must differ from the stale retained value %d", *stale.ScoreComputed)
	}
	if got, want := *cleared.Version, *stale.Version+1; got != want {
		t.Fatalf("version after clearing the override = %d, want %d", got, want)
	}
	if cleared.ScoreOverrideReason != nil || cleared.ScoreComputed != nil || cleared.Score != *set.ScoreComputed {
		t.Fatalf("cleared lead = %+v, want the machine score %d and no override", cleared, *set.ScoreComputed)
	}
}
