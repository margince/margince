// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type morningSelection struct {
	body    strings.Builder
	seen    map[string]bool
	shown   int
	partial bool
}

func (w *briefGenerateWorker) morningMessage(ctx context.Context, run briefs.BriefRun, now time.Time, words mailcopy.Copy) (string, bool, error) {
	service := newAttentionService(w.pool, approvals.NewService(InstallationDB(w.pool)), func() time.Time { return now })
	day, err := service.Worklist(ctx, "mine", "", ids.Nil, 50, "")
	if err != nil {
		return "", false, err
	}
	settings, err := w.users.MyDelivery(ctx)
	if err != nil {
		return "", false, err
	}
	chosen := morningSelection{seen: map[string]bool{}, partial: len(day.SourcesUnavailable) > 0}
	fmt.Fprintf(&chosen.body, "%s%s\n\n", words.MorningHeading, run.LocalDay.Format(mailcopy.DateLayout))
	scope := settings.ScopeChoice()
	if scope != identity.RoutineTeam {
		if scope == identity.RoutineCombined {
			chosen.body.WriteString(words.MailPersonal + "\n\n")
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
				chosen.body.WriteString(words.MailPersonal + "\n\n")
				chosen.append(day.Queue, 3, settings, w.mail.PublicBaseURL, words)
			}
			w.log.WarnContext(ctx, "team content unavailable; delivering personal content", "cause", err)
		}
	}
	return chosen.finish(day, w.mail.PublicBaseURL, words)
}

func (s *morningSelection) finish(day crmcontracts.Worklist, base string, words mailcopy.Copy) (string, bool, error) {
	if s.partial {
		s.body.WriteString(words.MailCoverage + "\n\n")
	}
	if s.shown == 0 && !s.partial && len(day.Queue) == 0 && day.Calendar != nil && *day.Calendar == "connected" {
		s.body.WriteString(words.MorningQuiet + "\n\n")
	}
	if s.shown == 0 && !s.partial && (day.Calendar == nil || *day.Calendar != "connected") {
		return "", false, nil
	}
	mailcopy.Link(&s.body, base, mailcopy.BriefMorningFragment, words.MorningOpenDay)
	return s.body.String(), s.shown > 0, nil
}

func (w *briefGenerateWorker) teamMorning(ctx context.Context, service *attention.Service, chosen *morningSelection, settings identity.Delivery, words mailcopy.Copy) error {
	members, cut, err := w.users.DeliveryTeam(ctx, settings)
	if err != nil {
		return err
	}
	chosen.body.WriteString(words.MailTeam + "\n\n")
	remaining := 3
	for _, member := range members {
		if member.Invited {
			chosen.partial = true
			continue
		}
		scope := string(crmcontracts.WorklistScopeTeam)
		if auth.TeamWeekReachOf(ctx) == auth.ReachesEveryTeam {
			scope = string(crmcontracts.WorklistScopeAll)
		}
		day, err := service.Worklist(ctx, scope, "", member.UserID, 10, "")
		if err != nil {
			return err
		}
		chosen.partial = chosen.partial || len(day.SourcesUnavailable) > 0
		remaining -= chosen.append(day.Queue, 1, settings, w.mail.PublicBaseURL, words)
		if remaining == 0 {
			break
		}
	}
	chosen.partial = chosen.partial || cut
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
		line := mailWorkItem(item, words)
		if line == "" {
			continue
		}
		s.body.WriteString(line + "\n\n")
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
