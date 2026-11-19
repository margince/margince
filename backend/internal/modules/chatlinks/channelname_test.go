// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func fakeChatClient(t *testing.T) *dispact.Client {
	t.Helper()
	srv := httptest.NewServer(dispacttest.New("team"))
	t.Cleanup(srv.Close)
	return dispact.New(srv.Client(), srv.URL, "gwapp_test")
}

func createFor(t *testing.T, chat Chat, id ids.UUID, name string) (dispact.Channel, error) {
	t.Helper()
	return resolveChannel(context.Background(), chat, RecordDeal, id, name, LinkRequest{Mode: ModeCreate})
}

func TestASecondDealWithTheSameNameGetsItsOwnChannel(t *testing.T) {
	chat := fakeChatClient(t)
	first, second := ids.NewV7(), ids.NewV7()
	if _, err := createFor(t, chat, first, "Brandt renewal"); err != nil {
		t.Fatalf("first deal: %v", err)
	}
	ch, err := createFor(t, chat, second, "Brandt renewal")
	if err != nil {
		t.Fatalf("second deal with the same name = %v, want a channel", err)
	}
	full := second.String()
	if want := "Brandt renewal " + full[len(full)-6:]; ch.Name != want {
		t.Errorf("second channel named %q, want %q", ch.Name, want)
	}
}

func TestALongTakenNameStaysWithinTheChatsLimit(t *testing.T) {
	chat := fakeChatClient(t)
	long := strings.Repeat("Ä", channelNameLimit+20)
	if _, err := createFor(t, chat, ids.NewV7(), long); err != nil {
		t.Fatalf("first deal: %v", err)
	}
	ch, err := createFor(t, chat, ids.NewV7(), long)
	if err != nil || len([]rune(ch.Name)) > channelNameLimit {
		t.Fatalf("second long name = %q (%d characters), %v; want a channel within %d", ch.Name, len([]rune(ch.Name)), err, channelNameLimit)
	}
}

func TestANameTheChatsRulesRefuseIsDerivedFromTheRecord(t *testing.T) {
	chat := fakeChatClient(t)
	id := ids.NewV7()
	ch, err := createFor(t, chat, id, "dm: Brandt")
	full := id.String()
	if err != nil || ch.Name != "deal-"+full[len(full)-6:] {
		t.Fatalf("a refused name = %q, %v; want deal-%s", ch.Name, err, full[len(full)-6:])
	}
}

func TestANameRefusedTwiceSaysToRenameNotToTryAgain(t *testing.T) {
	chat := fakeChatClient(t)
	id := ids.NewV7()
	full := id.String()
	for _, name := range []string{"Brandt renewal", "Brandt renewal " + full[len(full)-6:]} {
		if _, err := createFor(t, chat, ids.NewV7(), name); err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
	}
	_, err := createFor(t, chat, id, "Brandt renewal")
	if !errors.Is(err, ErrChannelNameRefused) || errors.Is(err, ErrChatUnreachable) {
		t.Fatalf("a name refused twice = %v, want ErrChannelNameRefused only", err)
	}
	rec := httptest.NewRecorder()
	writeErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "channel_name_refused") ||
		strings.Contains(rec.Body.String(), "Try again") {
		t.Errorf("answer = %d %s, want 422 channel_name_refused without a retry", rec.Code, rec.Body.String())
	}
}

func TestABlankNameIsDerivedFromTheRecord(t *testing.T) {
	chat := fakeChatClient(t)
	id := ids.NewV7()
	ch, err := createFor(t, chat, id, "   ")
	if want := "deal-" + idSuffix(id); err != nil || ch.Name != want {
		t.Fatalf("a blank name = %q, %v; want %s", ch.Name, err, want)
	}
	if got := fallbackName(RecordDeal, id, "", &dispact.CodeError{Code: dispact.CodeNameTaken}); got != "deal-"+idSuffix(id) {
		t.Errorf("a taken blank name falls back to %q, want deal-%s", got, idSuffix(id))
	}
}
