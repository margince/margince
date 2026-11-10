// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type previewSubject struct {
	kind string
	id   ids.UUID
}

func (s *Store) withReadablePreviewSubjects(ctx context.Context, preview SendPreview) (SendPreview, error) {
	preview.readableSubjects = make(map[previewSubject]bool)
	for _, decision := range preview.Decisions.Decisions {
		if decision.SubjectID.IsZero() || preview.NewContacts[correspondence.Fold(decision.Recipient.Email)] {
			continue
		}
		if decision.SubjectKind == "contact" || decision.SubjectKind == "lead" {
			preview.readableSubjects[previewSubject{kind: decision.SubjectKind, id: decision.SubjectID}] = false
		}
	}
	if len(preview.readableSubjects) == 0 {
		return preview, nil
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Record identity is optional; losing record access must not remove the
		// activity-create preview capability from an agent's tool catalog.
		for subject := range preview.readableSubjects {
			readCtx, err := auth.Unrecorded(ctx, subject.kind, principal.ActionRead)
			if err == nil {
				err = auth.EnsureReadable(readCtx, tx, subject.kind, subject.id)
			}
			if err != nil {
				if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
					continue
				}
				return fmt.Errorf("activities: read recipient evidence access: %w", err)
			}
			preview.readableSubjects[subject] = true
		}
		return nil
	})
	return preview, err
}
