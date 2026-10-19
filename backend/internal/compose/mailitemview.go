// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func mailWorkItem(item crmcontracts.WorklistItem, words mailcopy.Copy) string {
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
		lines = append(lines, words.MailDue+": "+item.DueAt.Format(mailcopy.DateLayout))
	}
	lines = append(lines, mailWorkAction(item, words))
	return strings.Join(lines, "\n")
}

func mailPreferences(body *strings.Builder, base string, words mailcopy.Copy) {
	body.WriteString("\n" + words.MailWhy + "\n")
	mailcopy.Link(body, base, mailcopy.PreferencesFragment, words.ManagePreferencesLabel)
}

func routineFooter(base string, words mailcopy.Copy) string {
	var body strings.Builder
	mailPreferences(&body, base, words)
	return body.String()
}

func writeMailItemLink(body *strings.Builder, item crmcontracts.WorklistItem, base string, words mailcopy.Copy) {
	if item.Subject == nil {
		return
	}
	fragment := ""
	switch item.Subject.Type {
	case crmcontracts.AttentionSubjectTypeDeal:
		fragment = mailcopy.DealRecordFragment
	case crmcontracts.AttentionSubjectTypeContact:
		fragment = mailcopy.ContactRecordFragment
	case crmcontracts.AttentionSubjectTypeCompany:
		fragment = mailcopy.CompanyRecordFragment
	case crmcontracts.AttentionSubjectTypeLead:
		fragment = mailcopy.LeadRecordFragment
	case crmcontracts.AttentionSubjectTypeProject:
		fragment = mailcopy.ProjectRecordFragment
	}
	if fragment != "" {
		mailcopy.Link(body, base, fragment+item.Subject.Id.String(), words.MailReview)
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
