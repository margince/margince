// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// SendExecutionPreviewer checks consent for the message a real send would render.
type SendExecutionPreviewer interface {
	ConsentGate
	decisionPreviewer
}

// PreviewSendExecution asks about the exact rendered send and rolls back every
// preparation write, including when the caller already holds a shared snapshot.
func (s *Store) PreviewSendExecution(ctx context.Context, origin SendOrigin, in SendEmailInput, gate SendExecutionPreviewer) (commsauthz.DecisionSet, error) {
	var decisions commsauthz.DecisionSet
	err := database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(ctx context.Context) error {
		return database.WithSavepoint(ctx, func(ctx context.Context) error {
			prepared, err := s.prepareRenderedSend(ctx, origin, in, gate, nil)
			if err != nil {
				return err
			}
			decisions, err = gate.Preview(ctx, prepared.Authorization())
			if err != nil {
				return err
			}
			return errPreviewFiled
		})
	})
	if err == errPreviewFiled {
		err = nil
	}
	return decisions, err
}
