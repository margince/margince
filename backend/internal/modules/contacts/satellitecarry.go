// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What happens to the rows OTHER modules keep about a subject when a merge
// retires it: the links in the subject's inbox, the lawful bases recorded for
// them, and the introductions colleagues asked for.
//
// A merge says the two records are the same human, so everything that human
// holds follows the survivor. Left behind, each of these rows names an id no
// read returns: an unsubscribe press withdraws the retired record while the
// survivor keeps receiving mail, a confirm link stops resolving, and an open
// introduction disappears from the survivor's page.
//
// Ports for the reason stopcarry.go gives: the owning module decides what a
// collision means, and contacts only asks, inside its own transaction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// ConsentSatelliteCarrier moves the consent-owned rows naming a retiring
// subject onto its survivor: withdrawal and preference links, confirm links,
// recorded lawful bases, qualifying events and double-opt-in history.
//
// Called after LockPair, so its own row locks come last in the merge's lock
// order (stop advisory locks, then the subject rows, then satellite rows).
type ConsentSatelliteCarrier interface {
	CarrySatellitesTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error
}

// IntroCarrier moves the introduction asks naming a retiring contact, on
// either of the two columns that can name one, onto the survivor.
type IntroCarrier interface {
	CarryIntrosTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error
}

// WithSatelliteCarriers wires the consent-side and introductions-side seams.
// Compose binds them to those modules' stores.
func (s *Store) WithSatelliteCarriers(consent ConsentSatelliteCarrier, intros IntroCarrier) *Store {
	s.consentCarrier = consent
	s.introCarrier = intros
	return s
}

// SatelliteCarrierNotWiredError maps to 422 for the reason
// StopCarrierNotWiredError does: nothing is broken, but this installation
// cannot merge this record without stranding rows another module owns.
type SatelliteCarrierNotWiredError struct {
	// Holding says what the source holds, in words an operator can act on.
	Holding string
}

func (e *SatelliteCarrierNotWiredError) Error() string {
	return "this record holds " + e.Holding + " and the seam that would move them onto the survivor " +
		"is not wired on this installation; merging would leave them on a record no read returns"
}

// FieldFault names the source record, the one holding the rows.
func (e *SatelliteCarrierNotWiredError) FieldFault() (field, code, message string) {
	return "source_id", "satellite_carrier_not_wired", e.Error()
}

// carryConsentSatellitesTx refuses only when an unwired seam would strand a
// row, the rule carryStopsTx spells for stops.
func (s *Store) carryConsentSatellitesTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	if s.consentCarrier != nil {
		return s.consentCarrier.CarrySatellitesTx(ctx, tx, from, to)
	}
	held, err := holdsAConsentSatellite(ctx, tx, from)
	if err != nil {
		return err
	}
	if held {
		return &SatelliteCarrierNotWiredError{Holding: "consent links or recorded lawful bases"}
	}
	return nil
}

// carryIntrosTx is the same rule for introduction asks.
func (s *Store) carryIntrosTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	if s.introCarrier != nil {
		return s.introCarrier.CarryIntrosTx(ctx, tx, from, to)
	}
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM intro_request WHERE contact_id = $1 OR through_contact_id = $1)`,
		from).Scan(&held); err != nil {
		return fmt.Errorf("contacts: checking whether the retiring contact is named by an introduction: %w", err)
	}
	if held {
		return &SatelliteCarrierNotWiredError{Holding: "introduction asks"}
	}
	return nil
}

// holdsAConsentSatellite is a bare EXISTS over tables contacts does not own,
// asked only to decide whether an unwired merge would destroy something.
// Whether a row still matters is consent's judgement, so every row counts.
//
// The table list mirrors consent's CarrySatellitesTx.
// Held by: TestEveryPortCarriesTheTablesItsRegisterEntryClaims (backend/gates/satellite_lifecycle_test.go)
func holdsAConsentSatellite(ctx context.Context, tx pgx.Tx, subject commsauthz.StopSubject) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM withdrawal_credential
		                WHERE ($1::uuid IS NOT NULL AND contact_id = $1) OR ($2::uuid IS NOT NULL AND lead_id = $2))
		    OR EXISTS (SELECT 1 FROM communication_basis
		                WHERE ($1::uuid IS NOT NULL AND contact_id = $1) OR ($2::uuid IS NOT NULL AND lead_id = $2))
		    OR EXISTS (SELECT 1 FROM preference_token WHERE contact_id = $1)
		    OR EXISTS (SELECT 1 FROM confirm_token WHERE contact_id = $1)
		    OR EXISTS (SELECT 1 FROM consent_qualifying_event WHERE contact_id = $1)
		    OR EXISTS (SELECT 1 FROM consent_doi_token WHERE contact_id = $1)`,
		zeroAsNull(subject.ContactID.UUID), zeroAsNull(subject.LeadID.UUID)).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("contacts: checking whether the retiring record holds consent links: %w", err)
	}
	return held, nil
}
