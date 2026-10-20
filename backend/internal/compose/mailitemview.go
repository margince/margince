// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func mailWorkItem(item crmcontracts.WorklistItem, words mailcopy.Copy, zone *time.Location) string {
	if item.Source == crmcontracts.WorklistItemSourceApproval && !slices.Contains(item.Actions, crmcontracts.WorklistItemActions("decide")) {
		return ""
	}
	label := ""
	if item.Subject != nil && item.Subject.Label != nil {
		label = *item.Subject.Label
	}
	if item.Title != nil && *item.Title != "" {
		label = *item.Title
	}
	if label == "" {
		return ""
	}
	lines := []string{mailcopy.OneLine(label)}
	if item.Detail != nil && *item.Detail != "" {
		lines = append(lines, mailcopy.OneLine(*item.Detail))
	}
	if item.CauseLabel != nil && *item.CauseLabel != "" {
		lines = append(lines, mailcopy.OneLine(*item.CauseLabel))
	}
	if item.Owner != nil && item.Owner.Label != nil {
		lines = append(lines, words.MailOwner+": "+mailcopy.OneLine(*item.Owner.Label))
	}
	if item.DueAt != nil {
		at := *item.DueAt
		if zone != nil {
			at = at.In(zone)
		}
		due := words.Instant(at)
		if item.Source == crmcontracts.WorklistItemSourceWeeklyCommitment {
			due = words.Date(*item.DueAt)
		}
		lines = append(lines, words.MailDue+": "+due)
	}
	lines = append(lines, mailWorkAction(item, words))
	return strings.Join(lines, "\n")
}

func routineFooterContent(base string, words mailcopy.Copy) emailview.Content {
	var content emailview.Content
	content.Paragraph(words.MailWhy)
	content.Action(words.ManagePreferencesLabel, mailcopy.Destination(base, mailcopy.PreferencesFragment))
	return content
}

func writeMailItemLink(body *emailview.Content, item crmcontracts.WorklistItem, base string, words mailcopy.Copy) {
	if item.Subject == nil {
		return
	}
	fragment := mailcopy.RecordFragment(string(item.Subject.Type), item.Subject.Id.String())
	if fragment != "" {
		body.Action(words.MailReview, mailcopy.Destination(base, fragment))
	}
}

func mailWorkAction(item crmcontracts.WorklistItem, words mailcopy.Copy) string {
	if item.PrimaryAction != nil && *item.PrimaryAction == crmcontracts.WorklistItemPrimaryActionComplete {
		return words.MailTask
	}
	if slices.Contains(item.Actions, crmcontracts.WorklistItemActionsReply) {
		return words.MailReply
	}
	if item.Source == crmcontracts.WorklistItemSourceMeeting {
		return words.MailPrepare
	}
	return words.MailReview
}
