// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What an anonymize keeps of a subject's stops.
//
// The approved send rules say an active opt-out never expires, and that
// deleting old proof never resets anyone to "permission unknown". An anonymize
// removes the addresses a stop is matched through, and a subject who comes back
// arrives as a new record. So every stop the subject holds, and every marketing
// withdrawal, ends up pinned to the addresses it covered. None is deleted.
//
// A pinned stop holds only the hash of its address (storekit.SuppressionHash),
// which the send check matches; the plaintext address leaves with the record.
// A merge binds a stop or withdrawal to its own record's addresses
// (recorded_for_addresses), and it is pinned to those alone. After a wrong merge the stop of one contact never
// lands on the addresses of the other.
//
// The contact and the lead arm share these statements; only the column that
// names the subject differs.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// stopSubjectColumn names the subject column of communication_suppression and
// contact_consent. Only the two constants below are ever passed.
type stopSubjectColumn string

const (
	stopsOfContact stopSubjectColumn = "contact_id"
	stopsOfLead    stopSubjectColumn = "lead_id"
)

// sourceKeptFromWithdrawal marks a stop the anonymize wrote from a withdrawal.
// A reader can tell it from a stop the subject asked for.
const sourceKeptFromWithdrawal = "withdrawal kept when the record was anonymized"

// keepContactStops keeps the stops of a contact and of every record merged
// into it. A merge copies the merged-away records' stops onto the survivor. An
// older merge may not have, so their own rows are pinned the same way.
func keepContactStops(ctx context.Context, tx pgx.Tx, id ids.UUID, addresses []string) error {
	rows, err := tx.Query(ctx, `SELECT id FROM contact WHERE merged_into_id = $1`, id)
	if err != nil {
		return fmt.Errorf("read the records merged into the subject: %w", err)
	}
	predecessors, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("read the records merged into the subject: %w", err)
	}
	for _, subject := range append([]ids.UUID{id}, predecessors...) {
		if err := keepStopsOnAddresses(ctx, tx, stopsOfContact, subject, addresses); err != nil {
			return err
		}
	}
	return nil
}

// liveStopToPin is one active stop of the subject and what it covers.
type liveStopToPin struct {
	id      ids.UUID
	address *string
	// bound is the list a merge bound the stop to. A copy made by code that did
	// not carry the list reads its original's. Nil covers every address the
	// subject holds.
	bound []string
}

// pinnedTo is the addresses one stop is pinned to.
func (s liveStopToPin) pinnedTo(addresses []string) []string {
	switch {
	case s.address != nil:
		return []string{*s.address}
	case s.bound != nil:
		return s.bound
	}
	return addresses
}

// keepStopsOnAddresses detaches every stop of one subject onto its addresses.
//
// Only an active stop is pinned. A stop that names its own address is pinned to
// its hash. A stop for the whole subject gets one row per address it covers. A
// stop with no address to pin to stays on the anonymized record, which still
// exists, so its evidence is kept. A lifted stop stays on the record too: it
// stops nothing, and a copy would only spread the address.
func keepStopsOnAddresses(ctx context.Context, tx pgx.Tx, column stopSubjectColumn, id ids.UUID, addresses []string) error {
	if err := pinWithdrawals(ctx, tx, column, id, addresses); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT s.id, s.address, coalesce(s.recorded_for_addresses, origin.recorded_for_addresses)
		  FROM communication_suppression s
		  LEFT JOIN communication_suppression origin ON origin.id = s.carried_from
		 WHERE s.%s = $1 AND s.revoked_at IS NULL
		 ORDER BY s.id`, column), id)
	if err != nil {
		return fmt.Errorf("read the subject's stops: %w", err)
	}
	stops, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (liveStopToPin, error) {
		var s liveStopToPin
		err := row.Scan(&s.id, &s.address, &s.bound)
		return s, err
	})
	if err != nil {
		return fmt.Errorf("read the subject's stops: %w", err)
	}
	for _, stop := range stops {
		if err := pinStop(ctx, tx, column, stop.id, stop.pinnedTo(addresses)); err != nil {
			return err
		}
	}
	return clearKeptStopAddresses(ctx, tx, column, id)
}

// addressHashes hashes each address the way the send check matches it.
func addressHashes(addresses []string) []string {
	hashes := make([]string, 0, len(addresses))
	for _, address := range addresses {
		hashes = append(hashes, storekit.SuppressionHash(address))
	}
	return hashes
}

// pinStop moves one stop off its subject onto the hash of its first address,
// and copies it onto each further one. A stop with no address to pin to is
// left where it is. A hard bounce keeps its address as well, because lifting
// one finds it by address (consent's LiftAddressStop). ON CONFLICT: one live
// bounce per address is unique, and an address that already has one is
// already stopped.
func pinStop(ctx context.Context, tx pgx.Tx, column stopSubjectColumn, stop ids.UUID, addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}
	hashes := addressHashes(addresses)
	if err := storekit.LockStopAddresses(ctx, tx, addresses, hashes); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO communication_suppression
		    (address, address_hash, address_key_id, kind, source, captured_by, decided_by_level, purpose_id,
		     recorded_at, carried_from, recorded_through)
		SELECT CASE WHEN s.kind = $4 THEN u.address END, u.hash, $5, s.kind, s.source, s.captured_by,
		       s.decided_by_level, s.purpose_id, s.recorded_at, s.id, s.recorded_through
		  FROM communication_suppression s
		 CROSS JOIN unnest($2::text[], $3::text[]) AS u(address, hash)
		 WHERE s.id = $1
		ON CONFLICT DO NOTHING`, stop, addresses[1:], hashes[1:], commsauthz.ReasonHardBounce, storekit.SuppressionKeyID()); err != nil {
		return fmt.Errorf("copy the subject's stop onto each address it covers: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE communication_suppression
		   SET %s = NULL, address = CASE WHEN kind = $4 THEN $2::text END, address_hash = $3,
		       address_key_id = $5, recorded_for_addresses = NULL
		 WHERE id = $1 AND revoked_at IS NULL`, column),
		stop, addresses[0], hashes[0], commsauthz.ReasonHardBounce, storekit.SuppressionKeyID()); err != nil {
		return fmt.Errorf("detach the subject's stop onto its address: %w", err)
	}
	return nil
}

// clearKeptStopAddresses empties the addresses of the stops that stay on the
// anonymized record as evidence: a lifted one, and one with nothing to pin to.
func clearKeptStopAddresses(ctx context.Context, tx pgx.Tx, column stopSubjectColumn, id ids.UUID) error {
	if err := storekit.LockSubjectsStopAddresses(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE communication_suppression SET address = NULL, recorded_for_addresses = NULL
		 WHERE %s = $1 AND (address IS NOT NULL OR recorded_for_addresses IS NOT NULL)`, column), id); err != nil {
		return fmt.Errorf("clear the addresses of the subject's kept stops: %w", err)
	}
	return nil
}

// keptWithdrawal is one group of a subject's marketing withdrawals that cover
// the same addresses, and the newest of them.
type keptWithdrawal struct {
	bound       []string
	withdrawnAt time.Time
}

// pinWithdrawals writes one marketing objection per address for a subject who
// withdrew a marketing purpose.
//
// A withdrawal lives on contact_consent, keyed by the subject, so a returning
// subject on a new record would not carry it. Broad, not per purpose: a
// withdrawal is read by class (consent's withdrawalCovers), and the classes are
// the ones commsauthz.MarketingPurposeClasses names. A withdrawal a merge bound
// to its record's addresses is pinned to those alone.
func pinWithdrawals(ctx context.Context, tx pgx.Tx, column stopSubjectColumn, id ids.UUID, addresses []string) error {
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT pc.recorded_for_addresses, max(coalesce(pc.captured_at, now()))
		  FROM contact_consent pc JOIN consent_purpose cp ON cp.id = pc.purpose_id
		 WHERE pc.%s = $1 AND pc.state = 'withdrawn' AND cp.class = ANY($2)
		 GROUP BY pc.recorded_for_addresses`, column),
		id, commsauthz.MarketingPurposeClasses())
	if err != nil {
		return fmt.Errorf("read the subject's marketing withdrawals: %w", err)
	}
	withdrawals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (keptWithdrawal, error) {
		var w keptWithdrawal
		err := row.Scan(&w.bound, &w.withdrawnAt)
		return w, err
	})
	if err != nil {
		return fmt.Errorf("read the subject's marketing withdrawals: %w", err)
	}
	for _, w := range withdrawals {
		covered := addresses
		if w.bound != nil {
			covered = w.bound
		}
		if err := keepWithdrawalOn(ctx, tx, covered, w.withdrawnAt); err != nil {
			return err
		}
	}
	return clearWithdrawalAddresses(ctx, tx, column, id)
}

// keepWithdrawalOn writes the marketing objection a withdrawal leaves on the
// hash of each address it covered.
func keepWithdrawalOn(ctx context.Context, tx pgx.Tx, addresses []string, withdrawnAt time.Time) error {
	if len(addresses) == 0 {
		return nil
	}
	hashes := addressHashes(addresses)
	if err := storekit.LockStopAddresses(ctx, tx, addresses, hashes); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO communication_suppression
		    (address_hash, address_key_id, kind, source, captured_by, decided_by_level, recorded_at, recorded_through)
		SELECT u.hash, $8, $2, $3, $4, $5, $6, $7 FROM unnest($1::text[]) AS u(hash)`,
		hashes, commsauthz.ReasonObjection, sourceKeptFromWithdrawal, by,
		string(commsauthz.LevelSubject), withdrawnAt, string(commsauthz.RecordedByMachine),
		storekit.SuppressionKeyID()); err != nil {
		return fmt.Errorf("keep the subject's marketing withdrawal on their addresses: %w", err)
	}
	return nil
}
