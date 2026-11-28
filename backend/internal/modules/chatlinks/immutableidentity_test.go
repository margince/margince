// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
)

func TestLegacyUUIDShapedSlugCannotBecomeAnImmutableChannelBinding(t *testing.T) {
	const identity = "018f3a1b-0000-7000-8000-00000000a001"
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		if r.URL.Path != "/api/bot/channels/id:"+identity {
			t.Errorf("channel lookup did not use the explicit identity namespace: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"ok":true,"channel":{"id":"` + identity + `","slug":"different-room","is_private":true,"bot_is_member":true}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	chat := dispact.NewCanonical(dispact.New(server.Client(), server.URL, "gwapp_fixture"))
	link := Link{Provider: ProviderDispact, ChannelSlug: identity}
	if _, err := LinkedChannel(context.Background(), chat, link); !errors.Is(err, ErrChannelIdentityNeedsReview) || reads != 0 {
		t.Fatalf("old slug was interpreted as another room's immutable ID: %v reads=%d", err, reads)
	}
	link.NativeIdentity = true
	channel, err := LinkedChannel(context.Background(), chat, link)
	if err != nil || channel.NativeID != identity || reads != 1 {
		t.Fatalf("explicitly confirmed identity was refused: %+v %v reads=%d", channel, err, reads)
	}
}
