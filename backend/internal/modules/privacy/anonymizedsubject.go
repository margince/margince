// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
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
//
// It reads the row FOR SHARE. An erasure changes the row before it takes back
// the notices naming it. So either the erasure waits and takes this
// transaction's notice back, or this read waits and sees the erasure.
func SubjectAnonymizedTx(ctx context.Context, tx pgx.Tx, subjectType string, id ids.UUID) (bool, error) {
	var named bool
	if err := tx.QueryRow(ctx, `SELECT `+subjectStillNamedSQL("$1::text", "$2::uuid", " FOR SHARE"),
		subjectType, id).Scan(&named); err != nil {
		return false, fmt.Errorf("privacy: reading whether the %s still names anyone: %w", subjectType, err)
	}
	return !named, nil
}

// NamedSubjectSQL holds while the contact or lead that typeExpr and idExpr name
// still names somebody. It is SubjectAnonymizedTx's answer, without the lock,
// for a query that must leave such subjects out.
func NamedSubjectSQL(typeExpr, idExpr string) string {
	return subjectStillNamedSQL(typeExpr, idExpr, "")
}

func subjectStillNamedSQL(typeExpr, idExpr, lock string) string {
	return fmt.Sprintf(`CASE %[1]s
		WHEN '%[3]s' THEN EXISTS (SELECT 1 FROM contact c WHERE c.id = %[2]s AND c.full_name IS DISTINCT FROM '%[5]s'%[7]s)
		WHEN '%[4]s' THEN EXISTS (SELECT 1 FROM lead ld WHERE ld.id = %[2]s AND ld.full_name IS DISTINCT FROM '%[6]s'%[7]s)
		ELSE false END`, typeExpr, idExpr, subjectContact, subjectLead, erasedName, anonymizedLeadName, lock)
}
