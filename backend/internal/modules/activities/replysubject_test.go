// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"os"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestReplySubjectsMatchTheComposer(t *testing.T) {
	content, err := os.ReadFile("testdata/replysubjects.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
		input, want, ok := strings.Cut(line, "|")
		if !ok {
			t.Fatalf("invalid subject fixture %q", line)
		}
		if got := ReplySubject(crmcontracts.ActivityKindEmail, input, "unused"); got != want {
			t.Errorf("subject %q: got %q, want %q", input, got, want)
		}
	}
	if got := ReplySubject(crmcontracts.ActivityKindNote, "A note", "New message"); got != "New message" {
		t.Errorf("non-email subject = %q", got)
	}
	if got := ReplySubject(crmcontracts.ActivityKindEmail, "", "Follow-up"); got != "Re: Follow-up" {
		t.Errorf("subjectless email = %q", got)
	}
}
