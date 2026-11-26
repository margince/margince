// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAnEditAndARevokeNameTheMessageTheyChange(t *testing.T) {
	at := time.Unix(1764000360, 0).UTC()
	base := Message{ID: "wamid.E", CounterpartyPhone: "491701234567", OriginalID: "wamid.A", At: at}

	edit := base
	edit.Kind, edit.Text = KindEdit, "corrected"
	got, err := ChatChange(edit)
	if err != nil {
		t.Fatalf("ChatChange(edit): %v", err)
	}
	if got.Kind != connector.ChatEdited || got.ConversationID != "491701234567" || got.IdempotencyKey != "wamid.A" ||
		got.Body != "corrected" || !got.Revision.Equal(at) || got.RevisionSeq != 0 {
		t.Errorf("edit = %+v, want an edit of wamid.A at the event time with no sequence", got)
	}

	revoke := base
	revoke.Kind = KindRevoke
	got, err = ChatChange(revoke)
	if err != nil || got.Kind != connector.ChatDeleted || got.IdempotencyKey != "wamid.A" {
		t.Errorf("revoke = %+v, %v, want a deletion of wamid.A", got, err)
	}
}

func TestAChangeThatChangesNothingIsSkipped(t *testing.T) {
	for name, msg := range map[string]Message{
		"a reaction":                {Kind: KindReaction, OriginalID: "wamid.A"},
		"an edit with no text":      {Kind: KindEdit, OriginalID: "wamid.A"},
		"a revoke naming no target": {Kind: KindRevoke},
	} {
		msg.CounterpartyPhone, msg.At = "491701234567", time.Unix(1, 0)
		if _, err := ChatChange(msg); !errors.Is(err, connector.ErrSkip) {
			t.Errorf("%s: ChatChange = %v, want a skip", name, err)
		}
	}
}
