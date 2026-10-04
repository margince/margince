// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheSettlementWindowKeepsTheRequestAndTheNewestAnswer(t *testing.T) {
	messages := make([]threadMessage, 10)
	for i := range messages {
		messages[i] = threadMessage{ID: ids.NewV7()}
	}
	newest := messages[9].ID

	got := windowKeeping(messages, newest, 4)

	want := []ids.UUID{messages[0].ID, messages[1].ID, messages[2].ID, newest}
	if len(got) != len(want) {
		t.Fatalf("the window holds %d messages, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("message %d is %v, want %v: the request, the oldest answers, then the newest", i, got[i].ID, id)
		}
	}
	if short := windowKeeping(messages[:3], newest, 4); len(short) != 3 {
		t.Errorf("a window under its size holds %d messages, want all 3", len(short))
	}
}
