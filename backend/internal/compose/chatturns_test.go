// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAlternatingTurnsJoinsARunOfOneRoleInOrderAndKeepsTheRest(t *testing.T) {
	got := alternatingTurns([]model.Message{
		{Role: chatRoleUser, Content: "context"},
		{Role: chatRoleUser, Content: "earlier question"},
		{Role: "assistant", Content: "earlier answer"},
		{Role: chatRoleUser, Content: "selection"},
		{Role: chatRoleUser, Content: "message"},
	})
	want := []model.Message{
		{Role: chatRoleUser, Content: "context\n\nearlier question"},
		{Role: "assistant", Content: "earlier answer"},
		{Role: chatRoleUser, Content: "selection\n\nmessage"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("alternatingTurns =\n%q\nwant\n%q", got, want)
	}
}

// requireTurnsInOrder fails unless messages carry every turn of want, in
// order, each inside a turn of its own role — which is what a replayed
// conversation promises once adjacent turns of one role are joined.
func requireTurnsInOrder(t *testing.T, messages, want []model.Message) {
	t.Helper()
	at, offset := 0, 0
	for _, turn := range want {
		for at < len(messages) {
			if messages[at].Role == turn.Role {
				if i := strings.Index(messages[at].Content[offset:], turn.Content); i >= 0 {
					offset += i + len(turn.Content)
					break
				}
			}
			at, offset = at+1, 0
		}
		if at == len(messages) {
			t.Fatalf("the request does not carry the %s turn %q in order after the ones before it:\n%+v", turn.Role, turn.Content, messages)
		}
	}
}

func TestAlternatingTurnsLeavesAnEmptyConversationEmpty(t *testing.T) {
	if got := alternatingTurns(nil); len(got) != 0 {
		t.Fatalf("alternatingTurns(nil) = %q, want no turns", got)
	}
}
