// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// MailSeatIdentitiesSQL binds private identities for matching readable mail only.
// Callers receive an expression, never aliases to return as another seat's profile.
func (s *OwnDomainStore) MailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return "", err
	}
	identities, err := mailSeatIdentities(ctx, tx)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(identities)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%d::jsonb", bind(encoded)), nil
}

func mailSeatIdentities(ctx context.Context, tx pgx.Tx) (map[string][]string, error) {
	rows, err := tx.Query(ctx, `SELECT u.id::text, u.email, '' FROM app_user u WHERE `+workforce.LiveMemberSQL("u")+`
 UNION SELECT i.user_id::text, i.value, i.source FROM capture_owner_identity i
 JOIN app_user u ON u.id = i.user_id WHERE i.kind = 'address' AND `+workforce.LiveMemberSQL("u")+`
 UNION SELECT c.user_id::text, c.account_label, '' FROM capture_connection c
 JOIN app_user u ON u.id = c.user_id WHERE c.archived_at IS NULL AND `+workforce.LiveMemberSQL("u")+` AND coalesce(c.account_label, '') <> ''`)
	if err != nil {
		return nil, fmt.Errorf("capture: reading mail identities: %w", err)
	}
	defer rows.Close()
	identities := map[string][]string{}
	for rows.Next() {
		var seat, label, source string
		if err := rows.Scan(&seat, &label, &source); err != nil {
			return nil, err
		}
		if address := ownReaderAddress(label, source); address != "" {
			identities[seat] = append(identities[seat], address)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return identities, nil
}
