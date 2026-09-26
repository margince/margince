// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ThreadMergeTx moves the grounds recorded for thread `from` onto thread `to`,
// when capture finds the two are one conversation (compose/threadmerge.go).
//
// A basis is scoped to the conversation it was earned in (basisAlreadyLive).
// The merged thread IS that conversation, so the basis goes with it; left on
// the retired key it would match nothing and the next send would record the
// same grounds again.
func ThreadMergeTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE communication_basis SET thread_key = $2 WHERE thread_key = $1`, from, to); err != nil {
		return fmt.Errorf("consent: moving recorded grounds to the thread they joined: %w", err)
	}
	return nil
}
