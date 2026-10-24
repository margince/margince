// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead's origin against Postgres: the catalog's refusals, the one score input
// the origin is, and the filter, sort and importer paths that read it.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scoreOf is the number the formula gives a lead with no signals, taken from the
// scorer itself so no expected value is hard-coded here.
func scoreOf(intent SourceIntent) int {
	return ScoreLeadDetail("", intent, nil, time.Time{}).Score
}

// keepOriginCatalogAsSeeded undoes what a test did to the origin list when it
// ends. A reset keeps the list, as it keeps every administered vocabulary, so
// a retired or extra origin would otherwise change the next test's world.
func keepOriginCatalogAsSeeded(t *testing.T, e *promoteConsentEnv) {
	t.Helper()
	t.Cleanup(func() {
		for _, statement := range []string{
			`UPDATE lead SET origin = NULL WHERE origin IN (SELECT key FROM lead_origin WHERE NOT system)`,
			`DELETE FROM lead_origin WHERE NOT system`,
			`UPDATE lead_origin SET active = true`,
		} {
			if _, err := e.owner.Exec(context.Background(), statement); err != nil {
				t.Errorf("restoring the origin list: %v", err)
			}
		}
	})
}

func newLead(t *testing.T, e *promoteConsentEnv, in CreateLeadInput) crmcontracts.Lead {
	t.Helper()
	if in.Source == "" {
		in.Source = "manual"
	}
	lead, _, err := e.store.CreateLead(e.ctx, in)
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	return lead
}

func TestLeadOriginCatalogRefusesWhatTheContractPromises(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	created, err := e.store.CreateLeadOrigin(e.ctx, CreateLeadOriginInput{Label: "Trade show", Intent: SourceIntentHigh})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Key != "trade_show" || created.Intent != crmcontracts.LeadSourceIntentLeadSourceIntentHigh || *created.System {
		t.Fatalf("created = %+v, want key trade_show, intent high, not system", created)
	}
	if _, err := e.store.CreateLeadOrigin(e.ctx, CreateLeadOriginInput{Label: "Trade Show"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("duplicate key err = %v, want ErrConflict", err)
	}
	if _, err := e.store.CreateLeadOrigin(e.ctx, CreateLeadOriginInput{Label: "Unset"}); err == nil {
		t.Error("the reserved filter word was accepted as a key")
	}

	newLead(t, e, CreateLeadInput{Origin: &created.Key})
	if err := e.store.DeleteLeadOrigin(e.ctx, ids.UUID(created.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("deleting an origin a lead carries err = %v, want ErrConflict", err)
	}

	list, err := e.store.ListLeadOrigins(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var referral crmcontracts.LeadOrigin
	for _, o := range list.Data {
		if o.Key == "referral" {
			referral = o
		}
		if o.Key == "trade_show" && (o.LeadCount == nil || *o.LeadCount != 1) {
			t.Errorf("trade_show lead_count = %v, want 1", o.LeadCount)
		}
	}
	if err := e.store.DeleteLeadOrigin(e.ctx, ids.UUID(referral.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("deleting a built-in err = %v, want ErrConflict", err)
	}

	rep := e.withGrants(map[string]principal.ObjectGrant{"lead": {Read: true}, "custom_field": {Read: true}})
	if _, err := e.store.CreateLeadOrigin(rep, CreateLeadOriginInput{Label: "Rep origin"}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("rep create err = %v, want ErrPermissionDenied", err)
	}
	if _, err := e.store.ListLeadOrigins(rep); err != nil {
		t.Errorf("rep list err = %v, want the read granted", err)
	}
}

// The score reads the origin and nothing else about where a lead came from.
// An imported lead scores the same as a typed one with the same origin. A lead
// with no origin scores neutral whatever its source says.
func TestOnlyTheOriginScoresALead(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	referral, cold, event := "referral", "cold_outreach", "event"
	sources := []string{"manual", "import", "crawl", "connector:apollo:a-1"}
	for i, source := range sources {
		lead := newLead(t, e, CreateLeadInput{Source: source, Origin: &referral})
		if want := scoreOf(SourceIntentHigh); lead.Score != want || want == 0 {
			t.Errorf("source %q with a referral origin scores %d, want %d (and not zero)", source, lead.Score, want)
		}
		bare := newLead(t, e, CreateLeadInput{Source: source})
		if want := scoreOf(SourceIntentNeutral); bare.Score != want {
			t.Errorf("source %q with no origin scores %d, want the neutral %d (case %d)", source, bare.Score, want, i)
		}
	}
	coldLead := newLead(t, e, CreateLeadInput{Origin: &cold})
	eventLead := newLead(t, e, CreateLeadInput{Origin: &event})
	if coldLead.Score != scoreOf(SourceIntentLow) || eventLead.Score != scoreOf(SourceIntentNeutral) {
		t.Errorf("cold %d event %d, want the low and neutral formula values", coldLead.Score, eventLead.Score)
	}

	id := ids.From[ids.LeadKind](ids.UUID(eventLead.Id))
	other := "crawl"
	if got, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{Source: &other}); err != nil || got.Score != eventLead.Score {
		t.Errorf("correcting the source moved the score to %d, %v; want it unchanged", got.Score, err)
	}
	moved, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{Origin: &referral})
	if err != nil || moved.Score != scoreOf(SourceIntentHigh) {
		t.Fatalf("moving to a referral origin scores %d, %v; want %d", moved.Score, err, scoreOf(SourceIntentHigh))
	}
	cleared, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{Clear: []string{"origin"}})
	if err != nil || cleared.Origin != nil || cleared.Score != scoreOf(SourceIntentNeutral) {
		t.Errorf("clearing the origin leaves origin %v score %d, %v; want none and the neutral value", cleared.Origin, cleared.Score, err)
	}
}

func TestAnOriginMustBeAnActiveCatalogKey(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	unknown, partner := "no_such_origin", "partner"
	var unknownErr *UnknownLeadOriginError
	if _, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{Source: "manual", Origin: &unknown}); !errors.As(err, &unknownErr) {
		t.Fatalf("create with an unknown origin err = %v, want UnknownLeadOriginError", err)
	}
	held := newLead(t, e, CreateLeadInput{Origin: &partner})

	list, err := e.store.ListLeadOrigins(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var origin crmcontracts.LeadOrigin
	for _, o := range list.Data {
		if o.Key == partner {
			origin = o
		}
	}
	retired := false
	if _, err := e.store.UpdateLeadOrigin(e.ctx, ids.UUID(origin.Id), UpdateLeadOriginInput{Active: &retired}); err != nil {
		t.Fatal(err)
	}
	var inactiveErr *InactiveLeadOriginError
	if _, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{Source: "manual", Origin: &partner}); !errors.As(err, &inactiveErr) {
		t.Errorf("create with a retired origin err = %v, want InactiveLeadOriginError", err)
	}
	// A lead already holding the retired key keeps it, and keeps its score.
	id := ids.From[ids.LeadKind](ids.UUID(held.Id))
	note := "met again in May"
	kept, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{Origin: &partner, OriginDetail: &note})
	if err != nil || kept.Origin == nil || *kept.Origin != partner || kept.Score != scoreOf(SourceIntentHigh) {
		t.Errorf("a lead keeping its retired origin = %v score %d, %v", kept.Origin, kept.Score, err)
	}
	referral := "referral"
	other := newLead(t, e, CreateLeadInput{Origin: &referral})
	if _, err := e.store.UpdateLead(e.ctx, ids.From[ids.LeadKind](ids.UUID(other.Id)), UpdateLeadInput{Origin: &partner}); !errors.As(err, &inactiveErr) {
		t.Errorf("moving a lead onto a retired origin err = %v, want InactiveLeadOriginError", err)
	}
}

func TestAHumanScoreOverrideOutlivesAnOriginChange(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	cold, referral := "cold_outreach", "referral"
	lead := newLead(t, e, CreateLeadInput{Origin: &cold})
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	override, reason := 77, "buyer confirmed budget on the phone"
	if _, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{Score: &override, ScoreOverrideReason: &reason}); err != nil {
		t.Fatal(err)
	}
	moved, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{Origin: &referral})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Score != override || moved.ScoreComputed == nil || *moved.ScoreComputed != scoreOf(SourceIntentHigh) {
		t.Errorf("score %d computed %v, want the human's %d kept and the machine value %d beside it",
			moved.Score, moved.ScoreComputed, override, scoreOf(SourceIntentHigh))
	}
}

// An importer's wording resolves to a catalog key by key or label, or stays in
// the note with the origin empty. A later file never replaces what the lead has.
func TestAnImportedOriginResolvesOrStaysAsANote(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	for wording, want := range map[string]string{"Referral": "referral", " cold outreach ": "cold_outreach", "LINKEDIN": "linkedin"} {
		lead := newLead(t, e, CreateLeadInput{OriginHint: wording})
		if lead.Origin == nil || *lead.Origin != want || lead.OriginDetail != nil {
			t.Errorf("hint %q filed as origin %v detail %v, want %q and no note", wording, lead.Origin, lead.OriginDetail, want)
		}
	}
	unmatched := newLead(t, e, CreateLeadInput{OriginHint: "Trade fair, hall 3"})
	if unmatched.Origin != nil || unmatched.OriginDetail == nil || *unmatched.OriginDetail != "Trade fair, hall 3" {
		t.Errorf("an unmatched hint = origin %v detail %v, want no origin and the wording as the note", unmatched.Origin, unmatched.OriginDetail)
	}
	if unmatched.Score != scoreOf(SourceIntentNeutral) {
		t.Errorf("an unmatched hint scores %d, want neutral", unmatched.Score)
	}

	id := ids.From[ids.LeadKind](ids.UUID(unmatched.Id))
	filled, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{OriginHint: "Partner"})
	if err != nil || filled.Origin == nil || *filled.Origin != "partner" || filled.Score != scoreOf(SourceIntentHigh) {
		t.Fatalf("a later file naming Partner = origin %v score %d, %v; want partner and the high score", filled.Origin, filled.Score, err)
	}
	again, err := e.store.UpdateLead(e.ctx, id, UpdateLeadInput{OriginHint: "Cold outreach"})
	if err != nil || again.Origin == nil || *again.Origin != "partner" || again.Version == nil || *again.Version != *filled.Version {
		t.Errorf("a second file replaced the origin: %v, %v; want partner untouched and no write", again.Origin, err)
	}
}

// The filter is one exact key bound as a parameter. LIKE metacharacters and a
// key that prefixes another match only themselves, and the list agrees with the
// origin each lead reads back.
func TestTheOriginFilterMatchesExactlyWhatTheLeadsRead(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	for key, label := range map[string]string{"event%": "Event percent", "event_x": "Event underscore", "back\\slash": "Backslash"} {
		if _, err := e.store.CreateLeadOrigin(e.ctx, CreateLeadOriginInput{Key: key, Label: label}); err != nil {
			t.Fatalf("create origin %q: %v", key, err)
		}
	}
	keys := []string{"event", "event%", "event_x", "back\\slash"}
	for _, key := range keys {
		newLead(t, e, CreateLeadInput{Origin: &key})
	}
	newLead(t, e, CreateLeadInput{})

	for _, key := range keys {
		got, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Origin: &key})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Origin == nil || *got[0].Origin != key {
			t.Errorf("origin=%q lists %d leads, want exactly the one that reads that origin", key, len(got))
		}
	}
	unset := "unset"
	got, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Origin: &unset})
	if err != nil || len(got) != 1 || got[0].Origin != nil {
		t.Errorf("origin=unset lists %d leads, %v; want the one lead without an origin", len(got), err)
	}

	sorted := "origin"
	all, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Sort: &sorted})
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, l := range all {
		if l.OriginLabel != nil {
			labels = append(labels, *l.OriginLabel)
		}
	}
	if !slices.IsSorted(labels) || len(labels) != len(keys) {
		t.Errorf("sorting by origin gave labels %v, want the %d labelled leads in label order", labels, len(keys))
	}
}
