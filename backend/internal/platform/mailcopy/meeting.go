// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// MeetingMessage keeps event facts and the management action together in each language.
func (c Copy) MeetingMessage(title, description, location, link string, start, end time.Time) emailview.Message {
	prefix, intro, action := "Reminder: ", "Your meeting is coming up.", "Reschedule or cancel"
	switch c.Language {
	case German:
		prefix, intro, action = "Erinnerung: ", "Ihr Termin steht an.", "Termin verschieben oder absagen"
	case Vietnamese:
		prefix, intro, action = "Nhắc cuộc hẹn: ", "Sắp đến cuộc hẹn của quý khách.", "Đổi lịch hoặc hủy cuộc hẹn"
	}
	unit := "min"
	if c.Language == German {
		unit = "Min."
	}
	if c.Language == Vietnamese {
		unit = "phút"
	}
	details := []string{OneLine(title), c.meetingInterval(start, end), fmt.Sprintf("%d %s", int(end.Sub(start)/time.Minute), unit)}
	if strings.TrimSpace(location) != "" {
		details = append(details, location)
	}
	if strings.TrimSpace(description) != "" {
		details = append(details, description)
	}
	var content emailview.Content
	content.Paragraph(intro)
	content.Paragraph(strings.Join(details, "\n"))
	content.Action(action, link)
	message := content.Message(prefix+OneLine(title)+" · "+c.Date(start), string(c.Language), intro)
	message.Text = content.String()
	return message
}
