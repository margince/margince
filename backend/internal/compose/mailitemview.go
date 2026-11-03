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

func mailWorkItem(item crmcontracts.WorklistItem, words mailcopy.Copy, zone *time.Location) (emailview.Item, bool) {
	if item.Source == crmcontracts.WorklistItemSourceApproval && !slices.Contains(item.Actions, crmcontracts.WorklistItemActions("decide")) {
		return emailview.Item{}, false
	}
	row := emailview.Item{Title: mailcopy.OneLine(mailWorkLabel(item)), Verb: mailWorkAction(item, words)}
	if strings.TrimSpace(row.Title) == "" {
		return emailview.Item{}, false
	}
	if item.Detail != nil && *item.Detail != "" {
		row.Lines = append(row.Lines, emailview.Line{Text: mailcopy.OneLine(*item.Detail)})
	}
	if item.CauseLabel != nil && *item.CauseLabel != "" {
		row.Lines = append(row.Lines, emailview.Line{Text: mailcopy.OneLine(*item.CauseLabel), Tone: emailview.Warning})
	}
	if item.Owner != nil && item.Owner.Label != nil {
		row.Lines = append(row.Lines, emailview.Line{Text: words.MailOwner + ": " + mailcopy.OneLine(*item.Owner.Label)})
	}
	if item.DueAt != nil {
		row.Lines = append(row.Lines, emailview.Line{Text: words.MailDue + ": " + mailWorkDue(item, *item.DueAt, words, zone), Tone: emailview.Info})
	}
	return row, true
}

// mailWorkLabel prefers the item's own title over its subject's label.
func mailWorkLabel(item crmcontracts.WorklistItem) string {
	if item.Title != nil && *item.Title != "" {
		return *item.Title
	}
	if item.Subject != nil && item.Subject.Label != nil {
		return *item.Subject.Label
	}
	return ""
}

// mailWorkDue names a weekly commitment's day, which has no clock time.
func mailWorkDue(item crmcontracts.WorklistItem, dueAt time.Time, words mailcopy.Copy, zone *time.Location) string {
	if item.Source == crmcontracts.WorklistItemSourceWeeklyCommitment {
		return words.Date(dueAt)
	}
	if zone != nil {
		dueAt = dueAt.In(zone)
	}
	return words.Instant(dueAt)
}

func routineFooterContent(base string, words mailcopy.Copy) emailview.Content {
	var content emailview.Content
	content.Footnote(words.MailWhy)
	content.FootnoteAction(words.ManagePreferencesLabel, mailcopy.Destination(base, mailcopy.PreferencesFragment))
	return content
}

func writeMailItemLink(body *emailview.Content, item crmcontracts.WorklistItem, base string, words mailcopy.Copy) {
	if item.Subject == nil {
		return
	}
	fragment := mailcopy.RecordFragment(string(item.Subject.Type), item.Subject.Id.String())
	if fragment != "" {
		body.ItemAction(words.MailReview, mailcopy.Destination(base, fragment))
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
