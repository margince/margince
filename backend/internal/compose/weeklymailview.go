// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// weeklyMessage builds the weekly mail, and says whether it carries a team's week.
func (w *weeklyGenerateWorker) weeklyMessage(ctx context.Context, review weekly.Review, settings identity.Delivery, now time.Time, words mailcopy.Copy) (emailview.Message, bool, error) {
	scope := settings.ScopeChoice()
	body, title, hasTeam := w.weeklyBaseMessage(ctx, review, settings, words)
	if scope != identity.RoutineTeam && !routineCombines(settings) {
		open, err := w.openTasksMail(ctx, review, now, words)
		if err != nil {
			w.log.WarnContext(ctx, "current weekly tasks unavailable", "cause", err)
			if len(body.Blocks) > 0 {
				body.Paragraph(words.MailCoverage)
			}
		} else {
			body.Append(open)
		}
	}
	morning, err := w.combinedMorningMessage(ctx, settings, now, words)
	if err != nil {
		return emailview.Message{}, false, err
	}
	if len(morning.Blocks) > 0 {
		if len(body.Blocks) == 0 {
			title = morning.Title
		} else {
			body.Heading(morning.Title)
		}
		body.Append(emailview.Content{Blocks: morning.Blocks})
	}
	if len(body.Blocks) == 0 {
		return emailview.Message{}, false, nil
	}
	if scope == identity.RoutineTeam && hasTeam {
		body.Action(words.MailTeamOpen, w.teamWeekURL(settings, review))
	} else if weekly.MailUseful(review) {
		body.Action(words.WeeklyFullWeek, mailcopy.Destination(w.mail.PublicBaseURL, mailcopy.WeeklyFragment(review.LocalWeekStart.Format(time.DateOnly))))
	}
	return body.Message(title, string(words.Language), body.Preview(words.WeeklyPreview)), hasTeam, nil
}

func (w *weeklyGenerateWorker) weeklyBaseMessage(ctx context.Context, review weekly.Review, settings identity.Delivery, words mailcopy.Copy) (emailview.Content, string, bool) {
	scope := settings.ScopeChoice()
	personal := weekly.MailMessage(review, "", words)
	title := personal.Title
	var body emailview.Content
	if scope != identity.RoutineTeam && weekly.MailUseful(review) {
		if scope == identity.RoutineCombined {
			body.Heading(words.MailPersonal)
		}
		body.Append(emailview.Content{Blocks: personal.Blocks})
	}
	var team emailview.Message
	if scope != identity.RoutinePersonal {
		var err error
		team, err = w.teamWeeklyMessage(ctx, review, settings, words)
		if err != nil {
			w.log.WarnContext(ctx, "team weekly unavailable; delivering available personal facts", "cause", err)
			if scope == identity.RoutineTeam && weekly.MailUseful(review) {
				body.Heading(words.MailPersonal)
				body.Append(emailview.Content{Blocks: personal.Blocks})
			}
			if len(body.Blocks) > 0 {
				body.Paragraph(words.MailMissing)
			}
		} else if len(team.Blocks) > 0 {
			if scope == identity.RoutineTeam {
				title = team.Title
			} else {
				body.Heading(team.Title)
			}
			body.Append(emailview.Content{Blocks: team.Blocks})
			if scope == identity.RoutineCombined {
				body.Action(words.MailTeamOpen, w.teamWeekURL(settings, review))
			}
		}
	}
	return body, title, len(team.Blocks) > 0
}

func (w *weeklyGenerateWorker) teamWeekURL(settings identity.Delivery, review weekly.Review) string {
	if settings.TeamID == nil {
		return ""
	}
	return mailcopy.Destination(w.mail.PublicBaseURL, mailcopy.TeamFragment(settings.TeamID.String(), review.LocalWeekStart.Format(time.DateOnly)))
}

func (w *weeklyGenerateWorker) teamWeeklyMessage(ctx context.Context, review weekly.Review, settings identity.Delivery, words mailcopy.Copy) (emailview.Message, error) {
	if _, _, err := w.users.DeliveryTeam(ctx, settings); err != nil {
		return emailview.Message{}, err
	}
	team, err := w.engine.TeamReview(ctx, *settings.TeamID, review.LocalWeekStart)
	if err != nil {
		return emailview.Message{}, err
	}
	if !weekly.TeamMailUseful(team) {
		return emailview.Message{}, nil
	}
	var body emailview.Content
	body.Paragraph(weekly.TeamMailSummary(team, words))
	body.Facts(weekly.TeamMailFacts(team, words)...)
	if team.RepsUnread > 0 {
		body.Paragraph(words.MailMissing)
	}
	for _, member := range team.Reps {
		if focus := weekly.TeamMailFocus(member, string(words.Language)); focus != "" {
			body.Paragraph(mailcopy.OneLine(member.DisplayName) + ": " + focus)
		}
		if member.HelpRequested > 0 {
			help, err := w.weeklyHelpMail(ctx, member, review.LocalWeekStart, words)
			if err != nil {
				return emailview.Message{}, err
			}
			body.Append(help)
		}
	}
	body.Append(teamCurrentMail(team, w.mail.PublicBaseURL, words))
	title := words.MailTeam + " · " + mailcopy.OneLine(team.TeamName) + " · " + words.DateRange(team.LocalWeekStart, team.LocalWeekStart.AddDate(0, 0, 6))
	return body.Message(title, string(words.Language), body.Preview(words.WeeklyPreview)), nil
}

func (w *weeklyGenerateWorker) combinedMorningMessage(ctx context.Context, settings identity.Delivery, now time.Time, words mailcopy.Copy) (emailview.Message, error) {
	if !routineCombines(settings) || (settings.MorningBrief != nil && *settings.MorningBrief == identity.DeliveryNone) {
		return emailview.Message{}, nil
	}
	_, allowed, err := routineWindowFor(ctx, w.users, now, settings)
	if err != nil || !allowed {
		return emailview.Message{}, err
	}
	worker := briefGenerateWorker{pool: w.pool, users: w.users, log: w.log, mail: BriefMailConfig(w.mail)}
	morning, useful, err := worker.morningMessage(ctx, now, words, true)
	if err != nil || !useful {
		return emailview.Message{}, err
	}
	return morning, nil
}
