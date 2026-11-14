// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"net/http/httptest"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
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
		chat := dispacttest.New("sweep-team")
		chat.SeedEveryChannel(dispacttest.Message{
			Seq: 1, UserName: "Role Sweep", Text: "We agree the role sweep.",
			PostedAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		sweepChatURL = httptest.NewServer(chat).URL
	})
	return sweepChatURL
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
