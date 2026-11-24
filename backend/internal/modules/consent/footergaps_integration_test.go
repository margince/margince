// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

func footerGaps(t *testing.T, e *channelConsentEnv, store *Store) map[ids.UUID]FooterGap {
	t.Helper()
	out := map[ids.UUID]FooterGap{}
	if err := store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		gaps, err := store.FooterGapsTx(e.ctx, tx)
		for _, gap := range gaps {
			out[gap.Entity] = gap
		}
		return err
	}); err != nil {
		t.Fatalf("reading the footer gaps: %v", err)
	}
	return out
}

func defaultEntity(t *testing.T, e *channelConsentEnv) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(e.ctx, `SELECT id FROM legal_entity WHERE is_default`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func recordHold(t *testing.T, e *channelConsentEnv, store *Store, set commsauthz.DecisionSet, intent ids.UUID, background bool) {
	t.Helper()
	if err := store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return RecordFooterHoldTx(e.ctx, tx, set, intent, background)
	}); err != nil {
		t.Fatalf("recording a footer hold: %v", err)
	}
}

// The gap is the answer the send gets: an obligation the entity's pack puts on
// every first message and the installation has not stated. Stating it ends
// the gap.
//
// Mutation: drop the DisclosuresFor answer from FooterGapsTx and this fails.
func TestAMissingCompanyDetailIsAGapUntilItIsStated(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "xa",
		messagingrules.Disclosure{Kind: messagingrules.ControllerIdentity},
		messagingrules.Disclosure{Kind: messagingrules.PrivacyContact})
	home := defaultEntity(t, e)
	stateParticulars(t, e, ControllerParticulars{LegalName: "Beispiel GmbH", PostalAddress: "Hauptstraße 1\n10115 Berlin"})
	gap := footerGaps(t, e, store)[home]
	if !slices.Equal(gap.Missing, []string{string(messagingrules.PrivacyContact)}) || !gap.IsDefault {
		t.Fatalf("the default entity's gap is %+v, want the privacy contact alone", gap)
	}

	stateParticulars(t, e, ControllerParticulars{LegalName: "Beispiel GmbH", PostalAddress: "Hauptstraße 1\n10115 Berlin", PrivacyContact: "privacy@beispiel.test"})
	if gap := footerGaps(t, e, store)[home]; len(gap.Missing) != 0 {
		t.Errorf("stated details still read as missing: %+v", gap)
	}
}

// A held send records its entity and the details it owed, split into sends a
// person pressed and background sends. A message pressed twice counts once. A
// marketing-only kind is owed per recipient, so the hold is what names it. An
// edit to another detail starts the count again and keeps the gap; stating the
// detail ends it.
//
// Mutation: skip the row in RecordFooterHoldTx, count log rows instead of
// intents, drop the occurred_at window, or drop the particularFor filter, and
// this fails.
func TestAHeldSendCountsInItsEntitysGapUntilTheDetailsChange(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "xb", messagingrules.Disclosure{Kind: messagingrules.ObjectionRoute, MarketingOnly: true})
	home := defaultEntity(t, e)
	if gap := footerGaps(t, e, store)[home]; len(gap.Missing) != 0 {
		t.Fatalf("an invoice owes no objection route, yet the gap is %+v", gap)
	}
	held := commsauthz.Decision{ReasonCode: commsauthz.ReasonFooterIncomplete, SendingEntity: home}
	held.Evidence.FooterFindings = []commsauthz.FooterFinding{{Kind: string(messagingrules.ObjectionRoute)}, {Kind: "unsubscribe_link"}}
	recordHold(t, e, store, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{held, held}}, ids.UUID{}, true)
	intent := ids.NewV7()
	for range 2 {
		recordHold(t, e, store, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{held}}, intent, false)
	}
	other := held
	other.ReasonCode = commsauthz.ReasonObjection
	recordHold(t, e, store, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{other}}, ids.UUID{}, false)

	gap := footerGaps(t, e, store)[home]
	if !slices.Equal(gap.Missing, []string{string(messagingrules.ObjectionRoute)}) {
		t.Errorf("missing %v, want the objection route alone: a stop link is no company detail", gap.Missing)
	}
	if gap.HeldInBackground != 1 || gap.HeldByPerson != 1 {
		t.Errorf("held %d in the background and %d by a person, want 1 and 1", gap.HeldInBackground, gap.HeldByPerson)
	}

	stateParticulars(t, e, ControllerParticulars{PrivacyContact: "privacy@beispiel.test"})
	gap = footerGaps(t, e, store)[home]
	if !slices.Equal(gap.Missing, []string{string(messagingrules.ObjectionRoute)}) || gap.HeldInBackground+gap.HeldByPerson != 0 {
		t.Errorf("after another detail changed the gap is %+v, want the objection route with a fresh count", gap)
	}

	stateParticulars(t, e, ControllerParticulars{PrivacyContact: "privacy@beispiel.test", ObjectionRoute: "Reply STOP."})
	if gap := footerGaps(t, e, store)[home]; len(gap.Missing) != 0 || gap.HeldInBackground+gap.HeldByPerson != 0 {
		t.Errorf("after the details changed the gap is %+v, want none", gap)
	}
}
