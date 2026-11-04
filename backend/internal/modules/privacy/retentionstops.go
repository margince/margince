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

// sourceKeptFromWithdrawal is the source of a stop written from a withdrawal at
// anonymization, so a reader can tell it from a stop the subject asked for.
const sourceKeptFromWithdrawal = "withdrawal kept when the record was anonymized"

// keepStopsOnAddresses detaches every stop of one subject onto its addresses.
//
// A stop that names its own address keeps it. A stop for the whole subject
// gets one row per address, because the subject asked about all of them. A
// stop with no address to pin to stays on the anonymized record, which still
// exists, so its evidence is kept.
func keepStopsOnAddresses(ctx context.Context, tx pgx.Tx, column stopSubjectColumn, id ids.UUID, addresses []string) error {
	if err := pinWithdrawals(ctx, tx, column, id, addresses); err != nil {
		return err
	}
	// The second and later addresses first, while the stop still names its
	// subject. ON CONFLICT: one live bounce per address is unique, and the
	// address that already has one is already stopped.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO communication_suppression
		    (address, kind, source, captured_by, decided_by_level, purpose_id, recorded_at, revoked_at, carried_from)
		SELECT u.addr, s.kind, s.source, s.captured_by, s.decided_by_level, s.purpose_id, s.recorded_at, s.revoked_at, s.id
		  FROM communication_suppression s
		 CROSS JOIN unnest($2::text[]) WITH ORDINALITY AS u(addr, n)
		 WHERE s.%[1]s = $1 AND s.address IS NULL AND u.n > 1
		ON CONFLICT DO NOTHING`, column), id, addresses); err != nil {
		return fmt.Errorf("copy the subject's stops onto each of their addresses: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE communication_suppression
		   SET %[1]s = NULL, address = coalesce(address, ($2::text[])[1])
		 WHERE %[1]s = $1 AND coalesce(address, ($2::text[])[1]) IS NOT NULL`, column), id, addresses); err != nil {
		return fmt.Errorf("detach the subject's stops onto their addresses: %w", err)
	}
	return nil
}

// pinWithdrawals writes one marketing objection per address for a subject who
// withdrew a marketing purpose.
//
// A withdrawal lives on contact_consent, keyed by the subject, so a returning
// subject on a new record would not carry it. Broad, not per purpose: a
// withdrawal is read by class (consent's withdrawalCovers), and the classes are
// the ones commsauthz.MarketingPurposeClasses names.
func pinWithdrawals(ctx context.Context, tx pgx.Tx, column stopSubjectColumn, id ids.UUID, addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}
	var withdrawnAt *time.Time
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT max(coalesce(pc.captured_at, now()))
		  FROM contact_consent pc JOIN consent_purpose cp ON cp.id = pc.purpose_id
		 WHERE pc.%s = $1 AND pc.state = 'withdrawn' AND cp.class = ANY($2)`, column),
		id, commsauthz.MarketingPurposeClasses()).Scan(&withdrawnAt); err != nil {
		return fmt.Errorf("read the subject's marketing withdrawal: %w", err)
	}
	if withdrawnAt == nil {
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level, recorded_at)
		SELECT u.addr, $2, $3, $4, $5, $6 FROM unnest($1::text[]) AS u(addr)`,
		addresses, commsauthz.ReasonObjection, sourceKeptFromWithdrawal, by,
		string(commsauthz.LevelSubject), *withdrawnAt); err != nil {
		return fmt.Errorf("keep the subject's marketing withdrawal on their addresses: %w", err)
	}
	return nil
}
