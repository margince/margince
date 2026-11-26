// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const quotedRefusal = "On Mon, 10 Nov 2026 at 09:00, Anna Berg <anna@example.test> wrote:\n> We will not go ahead with this project."

func TestTheClassifyPassReadsOnlyTheSendersCurrentMessage(t *testing.T) {
	reply := "Thanks, we will wait for your answer."
	if got := ClassifyBody(reply+"\n\n"+quotedRefusal, 1500); got != reply {
		t.Fatalf("classify body = %q, want the reply alone", got)
	}
	if got := ClassifyBody(quotedRefusal, 1500); got != "" {
		t.Fatalf("classify body of a mail that is only a quote = %q, want empty", got)
	}
}

func TestTheClassifyLimitAppliesToTheCurrentMessage(t *testing.T) {
	reply := strings.Repeat("a", 20)
	got := ClassifyBody(strings.Repeat("b", 3000)+"\n"+reply, 1500)
	if n := utf8.RuneCountInString(got); n != 1500 {
		t.Fatalf("classify body holds %d runes, want 1500", n)
	}
	if got := ClassifyBody(reply+"\n\n"+quotedRefusal+strings.Repeat(" more", 1000), 1500); got != reply {
		t.Fatalf("a long quote under a short reply left %q, want the reply alone", got)
	}
}
