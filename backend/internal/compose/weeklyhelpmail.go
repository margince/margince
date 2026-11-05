// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func (w *weeklyGenerateWorker) weeklyHelpMail(ctx context.Context, member weekly.TeamRep, week time.Time, words mailcopy.Copy) (emailview.Content, error) {
	var body emailview.Content
	body.Heading(words.MailHelp + " · " + mailcopy.OneLine(member.DisplayName))
	plan, err := weeklyPlanStore(w.pool).PlanFor(ctx, member.UserID, week)
	switch {
	case errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied):
		body.Paragraph(words.MailMissing)
	case err != nil:
		return emailview.Content{}, err
	default:
		shown := 0
		for _, commitment := range plan.Commitments {
			if commitment.HelpRequested == "" {
				continue
			}
			shown++
			body.Paragraph(mailcopy.OneLine(commitment.Label) + "\n" + mailcopy.Snippet(commitment.HelpRequested, 400))
			if commitment.ManagerResponse != "" {
				body.Paragraph(words.MailManagerResponse + ": " + mailcopy.Snippet(commitment.ManagerResponse, 400))
			}
		}
		if shown == 0 {
			body.Paragraph(words.MailMissing)
		}
	}
	return body, nil
}
