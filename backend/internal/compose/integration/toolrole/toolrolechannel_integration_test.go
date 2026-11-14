// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sweepChatSummaryReply is the episode site's answer to every chat message the
// sweep files.
const sweepChatSummaryReply = `{"relevant":true,"reason":"a decision","title":"Role sweep",` +
	`"summary":"The team agreed the role sweep.","decisions":[],"open_questions":[]}`

// sweepChatOnce starts the one fake Dispact the sweep's bot talks to.
var (
	sweepChatOnce sync.Once
	sweepChatURL  string
	// sweepChatBot serialises the bot's connect, which the installation holds once.
	sweepChatBot sync.Mutex
)

// sweepChat is the Dispact bot API as the channel verbs call it. Every channel
// is private with the bot in it, and every history holds one message, seq 1.
// It is posted after any link the sweep makes, because talk from before a link
// is not read.
func sweepChat() string {
	sweepChatOnce.Do(func() {
		srv := httptest.NewServer(http.HandlerFunc(serveSweepChat))
		sweepChatURL = srv.URL
	})
	return sweepChatURL
}

func serveSweepChat(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/bot/")
	slug, rest, _ := strings.Cut(strings.TrimPrefix(path, "channels/"), "/")
	var answer any
	switch {
	case path == "me":
		answer = map[string]any{"id": "sweep-bot", "name": "Sweep bot", "team_id": "sweep-team"}
	case strings.HasPrefix(path, "messages/"):
		answer = map[string]any{}
	case rest == "messages" && r.Method == http.MethodGet:
		messages := []map[string]any{}
		if r.URL.Query().Get("after") == "0" {
			messages = append(messages, map[string]any{"id": "m1", "seq": 1, "user_name": "Role Sweep",
				"text": "We agree the role sweep.", "posted_at": "2099-01-01T00:00:00Z"})
		}
		answer = map[string]any{"messages": messages}
	case rest == "messages":
		answer = map[string]any{"id": "m1", "seq": 2}
	case rest == "members":
		answer = map[string]any{"added": []string{}, "not_found": []string{}}
	default:
		answer = map[string]any{"slug": slug, "name": slug, "is_private": true, "is_archived": false, "bot_is_member": true}
	}
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func uniqueChannel() string { return "sweep-" + ids.NewV7().String()[24:] }

// channelFixtures fill the chat channel calls. Each takes a deal of its own,
// because an unlink or a link changes what the next call would find.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var channelFixtures = map[string]func(p *recordPool) map[string]any{
	"channel_link": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "deal", "id": p.take("channel_deal")}
	},
	"link_channel": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "deal", "id": p.take("unlinked_channel_deal"), "request_body": map[string]any{
			"mode": "existing", "channel": uniqueChannel(), "summarize_history": false,
		}}
	},
	"set_channel_verbosity": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "deal", "id": p.take("channel_deal"), "request_body": map[string]any{"verbosity": "silent"}}
	},
	"unlink_channel": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "deal", "id": p.take("channel_deal")}
	},
	"file_channel_message": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "deal", "id": p.take("channel_deal"), "request_body": map[string]any{"message": "1"}}
	},
}

// channelSeeders make deals with and without a channel, under the one Dispact
// bot the installation connects through its own route.
func channelSeeders(s sweepSeed) map[string]func() string {
	connected := func() {
		sweepChatBot.Lock()
		defer sweepChatBot.Unlock()
		if s.ownerScalar(`SELECT count(*)::text FROM channel_connection WHERE provider = 'dispact' AND archived_at IS NULL`) == "0" {
			s.call("POST", "/v1/channel-connections", integration.AnyMap{
				"provider": "dispact", "botToken": "gwapp_role_sweep", "address": sweepChat(),
			}, nil, nil)
		}
	}
	return map[string]func() string{
		"unlinked_channel_deal": func() string {
			connected()
			return s.create["deal"]()
		},
		"channel_deal": func() string {
			connected()
			deal := s.create["deal"]()
			s.call("POST", "/v1/channel-links/deal/"+deal, integration.AnyMap{
				"mode": "existing", "channel": uniqueChannel(), "summarize_history": false,
			}, nil, nil)
			return deal
		},
	}
}
