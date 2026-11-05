// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"strings"
	"testing"
)

func TestEveryProductPreviewExplainsItsPurposeInEachLanguage(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		words := For(language)
		for _, subject := range []string{
			words.MorningSubject,
			words.WeeklySubject,
			words.DigestSubject,
			words.NotificationSubject,
			words.ResetSubject,
			words.InviteSubject,
			words.RoomSubject,
			words.NoticeSubject,
			words.ConfirmRecordSubject,
			words.ConfirmConsentSubject,
			words.OptOutAckSubject,
		} {
			preview := words.Preview(subject)
			if preview == "" || preview == subject || strings.Contains(preview, "://") {
				t.Errorf("%s: subject %q has unhelpful preview %q", language, subject, preview)
			}
		}
	}
}
