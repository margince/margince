// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SignaturePhoneRepairPage advances over the unsupported phone rows selected for repair.
type SignaturePhoneRepairPage struct {
	Scanned []ids.UUID
	Changed int
}

const signaturePhoneSubjects = `p.source = @source AND p.archived_at IS NULL`

const (
	signaturePhoneLimitParam = "limit"
	signaturePhoneIDsParam   = "ids"
)

// A signature number must retain its evidence. A correction or another source's
// evidence is sufficient to keep it when the signature's evidence is withdrawn.
const signaturePhoneWithoutEvidence = `NOT EXISTS (SELECT 1 FROM ai_feedback
		WHERE subject_type = 'contact' AND subject_id = p.contact_id
		  AND claim_kind = 'profile_field' AND verdict = 'corrected'
		  AND claim_key = encode(sha256('profile_field:phone'::bytea), 'hex'))
	AND NOT EXISTS (SELECT 1 FROM contact_profile_field f
		WHERE f.contact_id = p.contact_id AND f.field = 'phone' AND f.value_key = p.phone)`

// Whole-contact restores replay an archive journal rather than a replacement,
// so they share this predicate with the signature retraction's revival.
const supportedSignaturePhone = `NOT (p.source = '` + enrichSource + `' AND ` + signaturePhoneWithoutEvidence + `)`

const unsupportedSignaturePhone = signaturePhoneSubjects + ` AND ` + signaturePhoneWithoutEvidence

// CountSignaturePhones bounds the repair estimate to limit plus one rows.
func CountSignaturePhones(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM contact_phone p WHERE `+signaturePhoneSubjects+` LIMIT @limit) phones`,
		pgx.StrictNamedArgs{auditKeySource: enrichSource, signaturePhoneLimitParam: limit + 1}).Scan(&count)
	return count, err
}

// RetractUnsupportedSignaturePhonesTx archives unsupported machine numbers while preserving a human's corrections.
func RetractUnsupportedSignaturePhonesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (SignaturePhoneRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return SignaturePhoneRepairPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id FROM contact_phone p
		WHERE `+unsupportedSignaturePhone+` AND p.id > @after ORDER BY p.id LIMIT @limit`,
		pgx.StrictNamedArgs{auditKeySource: enrichSource, "after": after, signaturePhoneLimitParam: limit})
	if err != nil {
		return SignaturePhoneRepairPage{}, fmt.Errorf("contacts: paging signature numbers: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return SignaturePhoneRepairPage{}, fmt.Errorf("contacts: reading signature numbers: %w", err)
	}
	page := SignaturePhoneRepairPage{Scanned: scanned}
	if len(scanned) == 0 {
		return page, nil
	}
	page.Changed, err = retractUnsupportedPhones(ctx, tx, scanned)
	return page, err
}

func retractUnsupportedPhones(ctx context.Context, tx pgx.Tx, scanned []ids.UUID) (int, error) {
	// Hold contacts before their numbers, as the signature writer and eraser do.
	if _, err := tx.Exec(ctx, `SELECT c.id FROM contact c
		WHERE c.id IN (SELECT contact_id FROM contact_phone WHERE id = ANY(@ids))
		ORDER BY c.id FOR UPDATE OF c`, pgx.StrictNamedArgs{signaturePhoneIDsParam: scanned}); err != nil {
		return 0, fmt.Errorf("contacts: holding contacts with signature numbers: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT p.id, p.contact_id, p.phone FROM contact_phone p
		WHERE p.id = ANY(@ids) AND `+unsupportedSignaturePhone+`
		ORDER BY p.id FOR UPDATE OF p`, pgx.StrictNamedArgs{signaturePhoneIDsParam: scanned, auditKeySource: enrichSource})
	if err != nil {
		return 0, fmt.Errorf("contacts: finding unsupported signature numbers: %w", err)
	}
	type number struct {
		ID        ids.UUID
		ContactID ids.ContactID
		Phone     string
	}
	numbers, err := pgx.CollectRows(rows, pgx.RowToStructByPos[number])
	if err != nil {
		return 0, fmt.Errorf("contacts: reading unsupported signature numbers: %w", err)
	}
	removed := make([]ids.UUID, 0, len(numbers))
	for _, n := range numbers {
		if err := withdrawSignatureNumber(ctx, tx, n.ContactID, n.Phone); err != nil {
			return 0, err
		}
		removed = append(removed, n.ID)
	}
	rows, err = tx.Query(ctx, `SELECT contact_id, 'phone' FROM contact_phone WHERE id = ANY(@ids) ORDER BY id`,
		pgx.StrictNamedArgs{signaturePhoneIDsParam: removed})
	if err != nil {
		return 0, fmt.Errorf("contacts: reading withdrawn signature numbers for audit: %w", err)
	}
	count, err := auditRetractions(ctx, tx, rows, "signature number has no supporting evidence")
	return int(count), err
}
