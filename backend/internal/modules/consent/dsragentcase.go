// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentsOpenCase answers the case an agent's repeat of an open request reaches. An
// agent's call carries no key a retry could repeat, and two cases for one request
// would each owe the subject an answer. A user may still file a second by hand.
// subject_ref is free text, and two subjects can share a name, so only a record's id
// identifies a repeat. A free-text retry can still open a second case.
func agentsOpenCase(ctx context.Context, tx pgx.Tx, in CreateDSRInput) (dsrRow, bool, error) {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return dsrRow{}, false, nil
	}
	subject := strings.TrimSpace(in.SubjectRef)
	if _, err := ids.Parse(subject); err != nil {
		return dsrRow{}, false, nil //nolint:nilerr // free text is not an error, and it identifies no repeat
	}
	if err := storekit.LockWriteIdentity(ctx, tx, "data_subject_request", in.Kind+":"+subject); err != nil {
		return dsrRow{}, false, err
	}
	out, err := scanDSR(tx.QueryRow(ctx, `SELECT `+dsrColumns+` FROM data_subject_request
		WHERE kind = $1 AND subject_ref = $2 AND status IN ('open', 'in_progress')
		ORDER BY created_at DESC LIMIT 1`, in.Kind, subject))
	if errors.Is(err, pgx.ErrNoRows) {
		return dsrRow{}, false, nil
	}
	if err != nil {
		return dsrRow{}, false, fmt.Errorf("consent: reading an open request: %w", err)
	}
	return out, true, nil
}
