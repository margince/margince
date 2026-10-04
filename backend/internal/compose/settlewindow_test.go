// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// messagesAt is a request and the messages after it, one minute apart.
func messagesAt(n int) []threadMessage {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	out := make([]threadMessage, n)
	for i := range out {
		out[i] = threadMessage{ID: ids.NewV7(), At: start.Add(time.Duration(i) * time.Minute)}
	}
	return out
}

func idsOf(messages []threadMessage) []ids.UUID {
	out := make([]ids.UUID, len(messages))
	for i, message := range messages {
		out[i] = message.ID
	}
	return out
}

// Past its size the window keeps the request, the newest answer, and the
// newest of the rest: a re-ask after our reply is the last word, and the
// middle of the exchange is what goes.
func TestTheSettlementWindowKeepsTheRequestTheAnswerAndTheLastWords(t *testing.T) {
	messages := messagesAt(10)
	answer := messages[5].ID

	got := idsOf(windowKeeping(messages, answer, 4))

	want := []ids.UUID{messages[0].ID, answer, messages[8].ID, messages[9].ID}
	if !slices.Equal(got, want) {
		t.Fatalf("the window holds %v, want the request, the answer and the two newest, in time order: %v", got, want)
	}
}

func TestASettlementWindowUnderItsSizeIsUnchanged(t *testing.T) {
	messages := messagesAt(3)
	if got := idsOf(windowKeeping(messages, messages[2].ID, 4)); !slices.Equal(got, idsOf(messages)) {
		t.Fatalf("the window holds %v, want all three in order", got)
	}
}

// Messages sent in the same second are ordered by id, as the thread read
// orders them, so one exchange reads the same on every pass.
func TestSettlementEvidenceBreaksATimeTieOnTheID(t *testing.T) {
	messages := messagesAt(3)
	messages[1].At = messages[2].At
	reversed := []threadMessage{messages[0], messages[2], messages[1]}

	inTimeOrder(reversed)

	if !slices.Equal(idsOf(reversed), idsOf(messages)) {
		t.Fatalf("a time tie reads %v, want it ordered by id: %v", idsOf(reversed), idsOf(messages))
	}
}
