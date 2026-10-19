// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func (w *weeklyGenerateWorker) weeklyMessage(ctx context.Context, review weekly.Review, settings identity.Delivery, now time.Time, words mailcopy.Copy) (string, error) {
	scope := settings.ScopeChoice()
	var body strings.Builder
	if scope != identity.RoutineTeam {
		if scope == identity.RoutineCombined {
			body.WriteString(words.MailPersonal + "\n\n")
		}
		body.WriteString(weekly.MailBody(review, w.mail.PublicBaseURL, words))
	}
	if scope != identity.RoutinePersonal {
		team, err := w.teamWeeklyMessage(ctx, review, settings, words)
		if err != nil {
			w.log.WarnContext(ctx, "team weekly unavailable; personal content remains available", "cause", err)
			if scope == identity.RoutineTeam {
				body.WriteString(words.MailPersonal + "\n\n" + weekly.MailBody(review, w.mail.PublicBaseURL, words))
			}
			body.WriteString("\n" + words.MailMissing + "\n")
		} else {
			body.WriteString(team)
		}
	}
	morning, err := w.combinedMorningMessage(ctx, settings, now, words)
	if err != nil {
		return "", err
	}
	if morning != "" {
		body.WriteString("\n\n" + morning)
	}
	mailPreferences(&body, w.mail.PublicBaseURL, words)
	return body.String(), nil
}

func (w *weeklyGenerateWorker) teamWeeklyMessage(ctx context.Context, review weekly.Review, settings identity.Delivery, words mailcopy.Copy) (string, error) {
	if _, _, err := w.users.DeliveryTeam(ctx, settings); err != nil {
		return "", err
	}
	team, err := w.engine.TeamReview(ctx, *settings.TeamID, review.LocalWeekStart)
	if err != nil {
		return "", err
	}
	var body strings.Builder
	fmt.Fprintf(&body, "\n%s · %s\n%s – %s\n\n", words.MailTeam, mailcopy.OneLine(team.TeamName), team.LocalWeekStart.Format(time.DateOnly), team.LocalWeekStart.AddDate(0, 0, 6).Format(time.DateOnly))
	if team.Summary != "" {
		body.WriteString(mailcopy.OneLine(team.Summary) + "\n\n")
	}
	if team.RepsUnread > 0 {
		body.WriteString(words.MailMissing + "\n\n")
	}
	for _, member := range team.Reps {
		if member.FocusLabel == "" && member.HelpRequested == 0 {
			continue
		}
		fmt.Fprintf(&body, "%s: %s\n", mailcopy.OneLine(member.DisplayName), mailcopy.OneLine(member.FocusLabel))
		if member.HelpRequested > 0 {
			fmt.Fprintf(&body, "%s: %d\n", words.MailHelp, member.HelpRequested)
		}
		body.WriteString("\n")
	}
	mailcopy.Link(&body, w.mail.PublicBaseURL, mailcopy.BriefWeeklyFragment, words.WeeklyFullWeek)

	return body.String(), nil
}

func (w *weeklyGenerateWorker) combinedMorningMessage(ctx context.Context, settings identity.Delivery, now time.Time, words mailcopy.Copy) (string, error) {
	if !routineCombines(settings) || (settings.MorningBrief != nil && *settings.MorningBrief == identity.DeliveryNone) {
		return "", nil
	}
	window, allowed, err := routineWindowFor(ctx, w.users, now, settings)
	if err != nil || !allowed {
		return "", err
	}
	worker := briefGenerateWorker{pool: w.pool, users: w.users, log: w.log, mail: BriefMailConfig(w.mail)}
	morning, useful, err := worker.morningMessage(ctx, briefs.BriefRun{LocalDay: window.Day}, now, words)
	if err != nil || !useful {
		return "", err
	}
	return morning, nil
}
