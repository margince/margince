// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The subject types a missed-receipt marker names.
const (
	subjectContact = "contact"
	subjectLead    = "lead"
)

// anonymizedLeadName is the name an erased or anonymized lead keeps, as the
// lead writers in erasure_leadtwins.go and retention_leadrecord.go spell it.
const anonymizedLeadName = "Anonymized Lead"

// SubjectAnonymizedTx answers whether a contact or lead no longer names anyone:
// erased, anonymized by a retention policy, or deleted. Nothing may be raised
// about such a subject again.
func SubjectAnonymizedTx(ctx context.Context, tx pgx.Tx, subjectType string, id ids.UUID) (bool, error) {
	var query, tombstone string
	switch subjectType {
	case subjectContact:
		query, tombstone = `SELECT full_name IS DISTINCT FROM $2 FROM contact WHERE id = $1`, erasedName
	case subjectLead:
		query, tombstone = `SELECT full_name IS DISTINCT FROM $2 FROM lead WHERE id = $1`, anonymizedLeadName
	default:
		return false, fmt.Errorf("privacy: %q is not a subject that can be erased", subjectType)
	}
	var named bool
	err := tx.QueryRow(ctx, query, id, tombstone).Scan(&named)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("privacy: reading whether the %s still names anyone: %w", subjectType, err)
	}
	return !named, nil
}
