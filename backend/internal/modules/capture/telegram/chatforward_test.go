// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

import (
	"testing"
	"time"
)

func TestAForwardOriginNamesTheAuthorOnlyForAUser(t *testing.T) {
	user := chatForward(&telegramForwardOrigin{Type: "user", Date: 1764000000, SenderUser: telegramUser{ID: 6002, FirstName: "Anna"}})
	if user == nil || user.Author == nil || user.Author.UserID != "6002" || user.Author.DisplayName != "Anna" {
		t.Fatalf("user origin = %+v", user)
	}
	if !user.OriginalAt.Equal(time.Unix(1764000000, 0)) {
		t.Errorf("original time = %v", user.OriginalAt)
	}
	hidden := chatForward(&telegramForwardOrigin{Type: "hidden_user", Date: 1764000000})
	if hidden == nil || hidden.Author != nil {
		t.Fatalf("hidden origin = %+v", hidden)
	}
	if chatForward(nil) != nil {
		t.Fatal("a message nobody forwarded carries a forward")
	}
}
