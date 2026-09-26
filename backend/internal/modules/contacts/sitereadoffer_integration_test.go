// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The offer slot on an onboarding read: stamped to whoever it was offered to,
// replaced whole by every answered message, audited when it changes, and blind
// to a read that is not an onboarding one.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const offerSlotKey = "conversation_offer"

func legalNameSiteReadOffer() *SiteReadOffer {
	return &SiteReadOffer{
		Field: "legal_name", Value: "Voltaq Systems GmbH", SourceIDs: []string{"S1"},
		TurnDigest: "turn-digest", DraftVersion: 3,
	}
}

// The slot keeps what the offer said and stamps it to the caller, whatever
// OfferedTo the offer arrived with: the server, not the conversation, says
// whom Margince asked.
func TestAnOfferIsStampedToTheHumanItWasMadeTo(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	readID := startOnboardingRead(ctx, t, e)

	offered := legalNameSiteReadOffer()
	offered.OfferedTo = "human:somebody-else"
	if err := e.store.ReplaceSiteReadOffer(ctx, readID, offered, nil); err != nil {
		t.Fatalf("record the offer: %v", err)
	}
	standing, err := e.store.StandingSiteReadOffer(ctx, readID)
	if err != nil {
		t.Fatalf("read the standing offer: %v", err)
	}
	want := *legalNameSiteReadOffer()
	want.OfferedTo = "human:" + e.rep.String()
	if standing == nil || standing.Field != want.Field || standing.Value != want.Value ||
		standing.OfferedTo != want.OfferedTo || standing.DraftVersion != want.DraftVersion ||
		standing.TurnDigest != want.TurnDigest {
		t.Fatalf("standing offer = %+v, want %+v", standing, want)
	}
	if offered.OfferedTo != "human:somebody-else" {
		t.Errorf("recording the offer rewrote the caller's own value to %q", offered.OfferedTo)
	}
	if n := countAuditRowsHolding(ctx, t, e.store, "site_read", readID, offerSlotKey); n != 1 {
		t.Errorf("recording an offer wrote %d audit rows, want 1", n)
	}
}

// Another human's conversation holds no offer for this one: a yes grants only
// what Margince offered to whoever is saying it.
func TestAnotherHumansOfferDoesNotStandForTheCaller(t *testing.T) {
	e := setupDedupe(t)
	readID := startOnboardingRead(e.as(), t, e)
	if err := e.store.ReplaceSiteReadOffer(e.as(), readID, legalNameSiteReadOffer(), nil); err != nil {
		t.Fatalf("record the offer: %v", err)
	}
	standing, err := e.store.StandingSiteReadOffer(asOfferColleague(t, e), readID)
	if err != nil || standing != nil {
		t.Fatalf("the colleague's standing offer = (%+v, %v), want none", standing, err)
	}
}

// Accepting an offer and clearing the slot is audited with the offer the yes
// accepted; clearing a slot that is already empty changes nothing and writes
// nothing.
func TestClearingTheSlotNamesTheAcceptedOfferAndAnEmptySlotStaysQuiet(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	readID := startOnboardingRead(ctx, t, e)

	if err := e.store.ReplaceSiteReadOffer(ctx, readID, nil, nil); err != nil {
		t.Fatalf("clear an empty slot: %v", err)
	}
	if n := countAuditRowsHolding(ctx, t, e.store, "site_read", readID, offerSlotKey); n != 0 {
		t.Fatalf("clearing an empty slot wrote %d audit rows, want none", n)
	}

	if err := e.store.ReplaceSiteReadOffer(ctx, readID, legalNameSiteReadOffer(), nil); err != nil {
		t.Fatalf("record the offer: %v", err)
	}
	// The yes accepts the offer as the slot holds it, stamp included.
	accepted, err := e.store.StandingSiteReadOffer(ctx, readID)
	if err != nil || accepted == nil {
		t.Fatalf("the standing offer = (%+v, %v), want the recorded one", accepted, err)
	}
	if err := e.store.ReplaceSiteReadOffer(ctx, readID, nil, accepted); err != nil {
		t.Fatalf("clear the accepted offer: %v", err)
	}
	standing, err := e.store.StandingSiteReadOffer(ctx, readID)
	if err != nil || standing != nil {
		t.Fatalf("after the yes the standing offer = (%+v, %v), want none", standing, err)
	}
	if got := acceptedOfferEvidence(ctx, t, e.store, readID); got.Field != accepted.Field || got.Value != accepted.Value {
		t.Errorf("the audit row names accepted offer %+v, want %+v", got, *accepted)
	}
}

// A read the caller cannot address as an onboarding read answers not-found on
// both sides of the slot, so its existence stays hidden.
func TestTheOfferSlotIsNotFoundOffAnOnboardingRead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyRead, _, err := e.store.StartSiteRead(ctx, seedCompanyForRead(ctx, t, e), "https://voltaq.test", "human:"+e.rep.String())
	if err != nil {
		t.Fatalf("start a company read: %v", err)
	}
	for name, readID := range map[string]ids.UUID{"unknown read": ids.NewV7(), "company read": companyRead.ID} {
		t.Run(name, func(t *testing.T) {
			if _, err := e.store.StandingSiteReadOffer(ctx, readID); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("StandingSiteReadOffer err = %v, want ErrNotFound", err)
			}
			if err := e.store.ReplaceSiteReadOffer(ctx, readID, legalNameSiteReadOffer(), nil); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("ReplaceSiteReadOffer err = %v, want ErrNotFound", err)
			}
		})
	}
}

// A slot holding something that is not an offer is reported as unreadable,
// never read as an empty slot a yes could then write over unnoticed.
func TestAnUnreadableSlotIsAnErrorNotAnEmptySlot(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	readID := startOnboardingRead(ctx, t, e)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE site_read SET conversation_offer = '"not an offer"'::jsonb WHERE id = $1`, readID)
		return err
	}); err != nil {
		t.Fatalf("corrupt the slot: %v", err)
	}
	if _, err := e.store.StandingSiteReadOffer(ctx, readID); err == nil {
		t.Error("StandingSiteReadOffer read an unreadable slot as an answer")
	}
	if err := e.store.ReplaceSiteReadOffer(ctx, readID, nil, nil); err == nil {
		t.Error("ReplaceSiteReadOffer replaced an unreadable slot without saying so")
	}
}

func startOnboardingRead(ctx context.Context, t *testing.T, e *dedupeEnv) ids.UUID {
	t.Helper()
	read, _, err := e.store.StartOnboardingSiteRead(ctx, "https://voltaq.test", "human:"+e.rep.String(),
		func(context.Context, pgx.Tx, SiteRead) error { return nil })
	if err != nil {
		t.Fatalf("start the onboarding read: %v", err)
	}
	return read.ID
}

// asOfferColleague is the workspace's second human, holding the same grants as
// the rep.
func asOfferColleague(t *testing.T, e *dedupeEnv) context.Context {
	t.Helper()
	rep, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("the rep's context carries no actor")
	}
	rep.ID, rep.UserID = "human:"+e.otherRep.String(), e.otherRep
	return principal.WithActor(e.as(), rep)
}

func acceptedOfferEvidence(ctx context.Context, t *testing.T, s *Store, readID ids.UUID) SiteReadOffer {
	t.Helper()
	var evidence struct {
		Accepted SiteReadOffer `json:"accepted_offer"`
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx,
			`SELECT evidence FROM audit_log
			  WHERE entity_type = 'site_read' AND entity_id = $1 AND action = 'update' AND evidence ? 'accepted_offer'
			  ORDER BY occurred_at DESC, id DESC LIMIT 1`, readID).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &evidence)
	}); err != nil {
		t.Fatalf("read the accepted-offer evidence: %v", err)
	}
	return evidence.Accepted
}
