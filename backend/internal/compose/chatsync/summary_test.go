// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAuthorEmailsSkipsAMessageWithNoAddress(t *testing.T) {
	messages := []dispact.Message{{Seq: 1, UserEmail: "rita@acme.test"}, {Seq: 2, UserName: "No Email"}}
	emails := authorEmails(messages)
	if len(emails) != 1 || emails[0] != "rita@acme.test" {
		t.Fatalf("authorEmails = %v, want only the matchable address; a nameless author is skipped, not passed over", emails)
	}
}

func TestAuthorEmailsAreLoweredInOrder(t *testing.T) {
	messages := []dispact.Message{{Seq: 1, UserEmail: " Rita@Acme.test "}, {Seq: 2, UserEmail: "clara@acme.test"}}
	emails := authorEmails(messages)
	if len(emails) != 2 || emails[0] != "rita@acme.test" || emails[1] != "clara@acme.test" {
		t.Fatalf("authorEmails = %v; want the two lowercased addresses", emails)
	}
}

func TestARestrictedAuthorsMessageIsDroppedAndMarked(t *testing.T) {
	messages := []dispact.Message{
		{Seq: 1, UserEmail: "rita@acme.test", Text: "keep"},
		{Seq: 2, UserEmail: "clara@acme.test", Text: "drop"},
	}
	kept, dropped := keepUnrestricted(messages, map[string]bool{"clara@acme.test": true})
	if !dropped || len(kept) != 1 || kept[0].Seq != 1 {
		t.Fatalf("keepUnrestricted kept %d (dropped=%t), want only seq 1 kept", len(kept), dropped)
	}
}

func TestAnEpisodeOfOnlyRestrictedAuthorsKeepsNothing(t *testing.T) {
	messages := []dispact.Message{{Seq: 1, UserEmail: "clara@acme.test"}}
	if kept, _ := keepUnrestricted(messages, map[string]bool{"clara@acme.test": true}); len(kept) != 0 {
		t.Fatalf("keepUnrestricted kept %d, want nothing when every author is restricted", len(kept))
	}
}

// refusingLane fails if the summary ever reaches a model, so a passed-over
// episode proves it never did.
type refusingLane struct{ t *testing.T }

func (l refusingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.t.Fatal("the model was asked about an episode that should have been passed over")
	return model.Response{}, nil
}

func TestSummarisePassesOverBeforeReachingTheModel(t *testing.T) {
	link := chatlinks.Link{ChannelName: "acme"}
	record := chatRecord{kind: "deal", name: "Acme renewal"}
	allRestricted := func(context.Context, []string) (map[string]bool, error) {
		return map[string]bool{"clara@acme.test": true}, nil
	}
	for name, messages := range map[string][]dispact.Message{
		"every author restricted": {{Seq: 1, UserEmail: "clara@acme.test", Text: "talk"}},
	} {
		_, err := summariser{lane: refusingLane{t}, restricted: allRestricted}.summarise(context.Background(), link, record, messages, false)
		if !errors.Is(err, errUnmatchableAuthor) {
			t.Errorf("%s: summarise = %v, want errUnmatchableAuthor", name, err)
		}
	}
}
