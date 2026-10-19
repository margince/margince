// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import "strings"

// Preview explains the purpose without repeating the subject or personal links.
func (c Copy) Preview(subject string) string {
	for _, item := range []struct{ subject, preview string }{
		{c.MorningSubject, c.MorningPreview},
		{c.WeeklySubject, c.WeeklyPreview},
		{c.DigestSubject, c.DigestPreview},
		{c.NotificationSubject, c.NotificationPreview},
		{c.ResetSubject, c.ResetPreview},
		{c.InviteSubject, c.InvitePreview},
		{c.RoomSubject, c.RoomPreview},
		{c.NoticeSubject, c.NoticePreview},
		{c.ConfirmRecordSubject, c.ConfirmRecordPreview},
		{c.ConfirmConsentSubject, c.ConfirmConsentPreview},
		{c.OptOutAckSubject, c.OptOutAckPreview},
	} {
		if strings.HasPrefix(subject, item.subject) {
			return item.preview
		}
	}
	return ""
}
