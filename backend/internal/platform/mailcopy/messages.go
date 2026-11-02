// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// AccountMessage builds the complete account message from its actual context.
func (c Copy) AccountMessage(invitation bool, inviter, company, link string) emailview.Message {
	var content emailview.Content
	title, intro, action, closing := c.ResetSubject, c.ResetIntro, c.ResetAction, c.ResetIgnore
	if invitation {
		title, intro, action, closing = c.InviteSubject, c.InviteIntro, c.InviteAction, c.InviteIgnore
		if inviter != "" && company != "" {
			content.Lead(fmt.Sprintf(c.InviteBy, OneLine(inviter), OneLine(company)))
		}
	}
	if !invitation || inviter == "" || company == "" {
		content.Lead(intro)
	}
	content.Action(action, link)
	content.Footnote(closing)
	message := content.Message(title, string(c.Language), c.Preview(title))
	message.Brand = company
	return message
}

// RoomMessage identifies the shared material and discloses personal access and activity recording.
func (c Copy) RoomMessage(title, inviter, company, link string, expires time.Time) emailview.Message {
	var content emailview.Content
	if inviter != "" && company != "" {
		content.Lead(fmt.Sprintf(c.RoomSharedBy, OneLine(inviter), OneLine(company)))
	} else if company != "" {
		content.Lead(OneLine(company))
	}
	content.Paragraph(c.RoomIntro)
	content.Heading(OneLine(title))
	content.Action(c.RoomAction, link)
	content.Note(fmt.Sprintf(c.RoomPersonal, c.Instant(expires)), emailview.Info)
	content.Note(c.RoomDisclosure, emailview.Neutral)
	content.Footnote(c.RoomIgnore)
	message := content.Message(c.RoomSubject+" · "+OneLine(title), string(c.Language), c.RoomPreview)
	message.Brand = company
	return message
}
