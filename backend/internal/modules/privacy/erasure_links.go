// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteSubjectRecordLinks removes every typed link that starts or ends on the
// contact. Anonymizing leaves the contact row standing, so a link left behind
// would still assert something about them. Both the Art. 17 erasure and the
// retention anonymize call it.
func deleteSubjectRecordLinks(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM record_link
		 WHERE (from_type = 'contact' AND from_id = $1) OR (to_type = 'contact' AND to_id = $1)`,
		contact); err != nil {
		return fmt.Errorf("privacy: clearing the subject's record links: %w", err)
	}
	return nil
}
