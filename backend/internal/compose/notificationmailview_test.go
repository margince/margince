// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestNoticeWordsCannotForgeMailStructure(t *testing.T) {
	notice := notices.Notice{Subject: "Quota reached\nFrom: finance@example.test", Body: "First line\nSubject: another message"}
	words := mailcopy.For("en")
	subject := notificationMailSubject(notice, words)
	body := notificationMailBody(notice, "https://example.test", words)
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatal("subject contains an injected header")
	}
	for _, line := range []string{"\nFrom:", "\nSubject:"} {
		if strings.Contains(body, line) {
			t.Fatal("notice forged a structural line")
		}
	}
	if !strings.Contains(body, "Quota reached") || !strings.Contains(body, "First line") {
		t.Fatal("flattening lost the actual content")
	}
}
