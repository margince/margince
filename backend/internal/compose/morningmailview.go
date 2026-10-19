// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"maps"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type morningSelection struct {
	body      emailview.Content
	title     string
	target    string
	action    string
	zone      *time.Location
	day       time.Time
	seen      map[string]bool
	shown     int
	partial   bool
	sectioned bool
}

func (w *briefGenerateWorker) morningMessage(ctx context.Context, now time.Time, words mailcopy.Copy, embedded bool) (emailview.Message, bool, error) {
	service := newAttentionService(w.pool, NewApprovalsService(InstallationDB(w.pool)), func() time.Time { return now })
	day, err := service.Worklist(ctx, "mine", "", ids.Nil, 50, "")
	if err != nil {
		return emailview.Message{}, false, err
	}
	settings, err := w.users.MyDelivery(ctx)
	if err != nil {
		return emailview.Message{}, false, err
	}
	window, _, err := routineWindowFor(ctx, w.users, now, settings)
	if err != nil {
		return emailview.Message{}, false, err
	}
	chosen := morningSelection{sectioned: !embedded, zone: window.Day.Location(), day: now.In(window.Day.Location()), seen: map[string]bool{}, partial: len(day.SourcesUnavailable) > 0}
	chosen.title = briefs.MailSubject(briefs.BriefRun{LocalDay: chosen.day}, words)
	chosen.target = mailcopy.Destination(w.mail.PublicBaseURL, mailcopy.BriefMorningFragment)
	chosen.action = words.MorningOpenDay
	scope := settings.ScopeChoice()
	if scope != identity.RoutineTeam {
		if scope == identity.RoutineCombined && chosen.sectioned {
			chosen.body.Heading(words.MailPersonal)
		}
		items := day.Queue
		if day.Focus != nil {
			items = day.Focus.Items
		}
		chosen.append(items, 3, settings, w.mail.PublicBaseURL, words)
	}
	if scope != identity.RoutinePersonal {
		if err := w.teamMorning(ctx, service, &chosen, settings, words); err != nil {
			chosen.partial = true
			if scope == identity.RoutineTeam {
				chosen.body.Heading(words.MailPersonal)
				chosen.append(day.Queue, 3, settings, w.mail.PublicBaseURL, words)
			}
			w.log.WarnContext(ctx, "team content unavailable; delivering personal content", "cause", err)
		}
	}
	if scope == identity.RoutineTeam && chosen.shown == 0 && !chosen.partial {
		return emailview.Message{}, false, nil
	}
	return chosen.finish(day, words)
}

func (s *morningSelection) finish(day crmcontracts.Worklist, words mailcopy.Copy) (emailview.Message, bool, error) {
	if s.partial {
		s.body.Paragraph(words.MailCoverage)
	}
	if s.shown == 0 && !s.partial && len(day.Queue) == 0 && day.Calendar != nil && *day.Calendar == "connected" {
		s.body.Paragraph(words.MorningQuiet)
	}
	if s.shown == 0 && !s.partial && (day.Calendar == nil || *day.Calendar != "connected") {
		return emailview.Message{}, false, nil
	}
	s.body.Action(s.action, s.target)
	return s.body.Message(s.title, string(words.Language), s.body.Preview(words.MorningPreview)), s.shown > 0, nil
}

func (w *briefGenerateWorker) teamMorning(ctx context.Context, service *attention.Service, chosen *morningSelection, settings identity.Delivery, words mailcopy.Copy) error {
	original := chosen
	staged := *chosen
	staged.body.Blocks = append([]emailview.Block{}, chosen.body.Blocks...)
	staged.seen = maps.Clone(chosen.seen)
	chosen = &staged
	members, cut, err := w.users.DeliveryTeam(ctx, settings)
	if err != nil {
		return err
	}
	team, err := w.users.DeliveryTeamName(ctx, settings)
	if err != nil {
		return err
	}
	if chosen.sectioned && settings.ScopeChoice() != identity.RoutineTeam {
		chosen.body.Heading(words.MailTeam + " · " + mailcopy.OneLine(team))
	}
	allowed := make(map[ids.UUID]bool, len(members))
	for _, member := range members {
		if member.Invited {
			chosen.partial = true
			continue
		}
		allowed[member.UserID] = true
	}
	scope := string(crmcontracts.WorklistScopeTeam)
	if auth.TeamWeekReachOf(ctx) == auth.ReachesEveryTeam {
		scope = string(crmcontracts.WorklistScopeAll)
	}
	cursor := ""
	remaining := 3
	for page := 0; remaining > 0; page++ {
		if page == 5 {
			chosen.partial = true
			break
		}
		day, err := service.Worklist(ctx, scope, "", ids.Nil, 100, cursor)
		if err != nil {
			return err
		}
		chosen.partial = chosen.partial || len(day.SourcesUnavailable) > 0 || cut
		for _, item := range day.Queue {
			if item.Owner == nil || item.Owner.Id == nil || !allowed[ids.UUID(*item.Owner.Id)] {
				continue
			}
			remaining -= chosen.append([]crmcontracts.WorklistItem{item}, 1, settings, w.mail.PublicBaseURL, words)
			if remaining == 0 {
				break
			}
		}
		if day.NextCursor == nil {
			break
		}
		cursor = *day.NextCursor
	}

	target := mailcopy.Destination(w.mail.PublicBaseURL, mailcopy.TeamFragment(settings.TeamID.String(), ""))
	if settings.ScopeChoice() == identity.RoutineTeam {
		chosen.title = words.MailTeam + " · " + mailcopy.OneLine(team) + " · " + words.Date(chosen.day)
		chosen.action, chosen.target = words.MailTeamDay, target
	} else {
		chosen.body.Action(words.MailTeamDay, target)
	}

	*original = *chosen
	return nil
}

func (s *morningSelection) append(items []crmcontracts.WorklistItem, limit int, settings identity.Delivery, base string, words mailcopy.Copy) int {
	shown := 0
	for _, item := range items {
		if s.seen[item.Id] {
			continue
		}
		if settings.CombineRoutine != nil && *settings.CombineRoutine && string(item.Source) == "notice" {
			continue
		}
		line := mailWorkItem(item, words, s.zone)
		if line == "" {
			continue
		}
		s.body.Paragraph(line)
		writeMailItemLink(&s.body, item, base, words)
		s.seen[item.Id] = true
		shown++
		s.shown++
		if shown == limit {
			break
		}
	}
	return shown
}
