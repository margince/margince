// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const editedMessageJSON = `{
	"update_id": 400,
	"edited_message": {
		"message_id": 7,
		"chat": {"id": 1001, "type": "private", "username": "annlee"},
		"from": {"id": 555, "username": "annlee", "first_name": "Ann", "last_name": "Lee"},
		"date": 1690000000,
		"edit_date": 1690000600,
		"text": "hello, edited"
	}
}`

const editedBusinessJSON = `{
	"update_id": 401,
	"edited_business_message": {
		"business_connection_id": "bc-1",
		"message_id": 9,
		"chat": {"id": 1001, "type": "private", "first_name": "Cora", "username": "cora"},
		"from": {"id": 1001, "first_name": "Cora", "username": "cora"},
		"date": 1690000000,
		"edit_date": 1690000900,
		"text": "still in stock?"
	}
}`

const deletedBusinessJSON = `{
	"update_id": 402,
	"deleted_business_messages": {
		"business_connection_id": "bc-1",
		"chat": {"id": 1001, "type": "private", "first_name": "Cora"},
		"message_ids": [9, 10]
	}
}`

func changesOf(t *testing.T, update string, link *BusinessLink) (connector.ChatBinding, []connector.ChatChange, error) {
	t.Helper()
	raw, err := BuildRawEnvelope("42", []byte(update))
	if err != nil {
		t.Fatalf("BuildRawEnvelope: %v", err)
	}
	return ParseChatChanges(raw, link)
}

func TestAnEditedMessageIsAnEditOfTheMessageItRestates(t *testing.T) {
	binding, changes, err := changesOf(t, editedMessageJSON, nil)
	if err != nil {
		t.Fatalf("ParseChatChanges: %v", err)
	}
	if binding.KeyScope != "42" || len(changes) != 1 {
		t.Fatalf("binding scope %q and %d changes, want the bot's scope and one change", binding.KeyScope, len(changes))
	}
	edit := changes[0]
	if edit.Kind != connector.ChatEdited || edit.ConversationID != "1001" || edit.IdempotencyKey != "7" || edit.Body != "hello, edited" {
		t.Errorf("change = %+v, want an edit of message 7 in chat 1001 with the new text", edit)
	}
	if want := time.Unix(1690000600, 0).UTC(); !edit.Revision.Equal(want) {
		t.Errorf("revision = %v, want the edit time %v", edit.Revision, want)
	}
	if edit.RevisionSeq != 400 || edit.Restated == nil || edit.Restated.RevisionSeq != 400 {
		t.Errorf("revision sequence = %d, want the edit update's id 400, so two edits of one second keep their order", edit.RevisionSeq)
	}
	if edit.Restated == nil || edit.Restated.Body != "hello, edited" || !edit.Restated.OccurredAt.Equal(time.Unix(1690000000, 0).UTC()) || !edit.Restated.Revision.Equal(edit.Revision) {
		t.Errorf("restated = %+v, want the whole message at its original time carrying the edit time", edit.Restated)
	}
}

func TestAnEditedBusinessMessageKeysOnTheAccountLikeTheOriginal(t *testing.T) {
	binding, changes, err := changesOf(t, editedBusinessJSON, approvedLink)
	if err != nil {
		t.Fatalf("ParseChatChanges: %v", err)
	}
	if binding.KeyScope != "42:biz:880001" || len(changes) != 1 || changes[0].IdempotencyKey != "9" {
		t.Errorf("binding scope %q and changes %+v, want the original's account scope and message 9", binding.KeyScope, changes)
	}
}

func TestADeletionNamesEveryRemovedMessageOfTheChat(t *testing.T) {
	_, changes, err := changesOf(t, deletedBusinessJSON, approvedLink)
	if err != nil {
		t.Fatalf("ParseChatChanges: %v", err)
	}
	var got []string
	for _, c := range changes {
		if c.Kind != connector.ChatDeleted || c.ConversationID != "1001" {
			t.Errorf("change = %+v, want a deletion in chat 1001", c)
		}
		got = append(got, c.IdempotencyKey)
	}
	if !slices.Equal(got, []string{"9", "10"}) {
		t.Errorf("deleted ids = %v, want [9 10]", got)
	}
}

func TestAnUpdateThatChangesNothingIsSkipped(t *testing.T) {
	for name, tc := range map[string]struct {
		update string
		link   *BusinessLink
	}{
		"a plain message":                 {telegramUpdateFixture, nil},
		"an edit with no edit time":       {`{"update_id":1,"edited_message":{"message_id":7,"chat":{"id":1001,"type":"private"},"from":{"id":555},"date":1,"text":"x"}}`, nil},
		"an edit in a group":              {`{"update_id":1,"edited_message":{"message_id":7,"chat":{"id":-5,"type":"supergroup"},"from":{"id":555},"date":1,"edit_date":2,"text":"x"}}`, nil},
		"a business edit on the bot link": {editedBusinessJSON, nil},
		"a deletion on the bot":           {deletedBusinessJSON, nil},
		"a deletion in a group":           {`{"update_id":1,"deleted_business_messages":{"business_connection_id":"bc-1","chat":{"id":-5,"type":"supergroup"},"message_ids":[1]}}`, approvedLink},
		"a deletion naming no message":    {`{"update_id":1,"deleted_business_messages":{"business_connection_id":"bc-1","chat":{"id":1001,"type":"private"},"message_ids":[]}}`, approvedLink},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := changesOf(t, tc.update, tc.link); !errors.Is(err, connector.ErrSkip) {
				t.Errorf("ParseChatChanges = %v, want a skip", err)
			}
		})
	}
}

func TestAnEditOrDeletionIsNotANewMessage(t *testing.T) {
	for _, update := range []string{editedMessageJSON, editedBusinessJSON, deletedBusinessJSON} {
		raw, err := BuildRawEnvelope("42", []byte(update))
		if err != nil {
			t.Fatal(err)
		}
		for _, link := range []*BusinessLink{nil, approvedLink} {
			if _, _, err := ParseChatMessage(raw, link); !errors.Is(err, connector.ErrSkip) {
				t.Errorf("ParseChatMessage = %v, want a skip: a change files no message", err)
			}
		}
	}
}

func TestEditsAndDeletionsAreScopedByTheirChatAndSender(t *testing.T) {
	for name, tc := range map[string]struct {
		update string
		want   []string
	}{
		"an edit names its sender":         {editedMessageJSON, []string{"555"}},
		"a business edit names its chat":   {editedBusinessJSON, []string{"1001"}},
		"a deletion names its chat":        {deletedBusinessJSON, []string{"1001"}},
		"a deletion in a group names none": {`{"update_id":1,"deleted_business_messages":{"business_connection_id":"bc","chat":{"id":-5,"type":"supergroup"},"message_ids":[1]}}`, nil},
		"a business edit with no sender":   {`{"update_id":1,"edited_business_message":{"business_connection_id":"bc","message_id":1,"chat":{"id":1001,"type":"private"},"date":1,"edit_date":2,"text":"x"}}`, nil},
		"an update carrying two kinds":     {`{"update_id":1,"message":{"message_id":1,"chat":{"id":1001,"type":"private"},"from":{"id":555},"date":1,"text":"x"},"edited_message":{"message_id":1,"chat":{"id":1001,"type":"private"},"from":{"id":555},"date":1,"edit_date":2,"text":"y"}}`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := InScopeSubjects([]byte(tc.update))
			if err != nil {
				t.Fatalf("InScopeSubjects: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("accounts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEveryBusinessUpdateKindNamesItsConnection(t *testing.T) {
	for _, update := range []string{businessUpdateJSON(customerFrom, "private", "1001"), editedBusinessJSON, deletedBusinessJSON} {
		id, isBusiness, err := BusinessConnectionIDOf([]byte(update))
		if err != nil || !isBusiness || id != "bc-1" {
			t.Errorf("BusinessConnectionIDOf = %q, %v, %v, want bc-1 for a business update", id, isBusiness, err)
		}
	}
	if _, isBusiness, _ := BusinessConnectionIDOf([]byte(editedMessageJSON)); isBusiness {
		t.Error("an edit of a message to the bot itself was read as a business update")
	}
}
