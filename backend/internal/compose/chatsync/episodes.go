// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// Episodes: a channel's talk, cut into stretches a note can summarise. No
// episode is stored. The link's history cursor marks the end of what was
// judged, so the next pull groups the messages after it again.

import (
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
)

// Quiet is how long a channel must be silent before its last stretch of talk
// is closed and judged.
const Quiet = 30 * time.Minute

// episode is one stretch of talk: consecutive messages with less than Quiet
// between them, all on one day. Thread replies are talk like any other, so a
// thread answered within the quiet window stays in its episode.
type episode struct {
	messages []dispact.Message
	closed   bool
}

func (e episode) firstSeq() int64 { return e.messages[0].Seq }
func (e episode) lastSeq() int64  { return e.messages[len(e.messages)-1].Seq }

// key is the episode's source_id on the note it files. It names the channel
// and the episode's first message, so linking the channel again files nothing
// twice.
func (e episode) key(slug string) string {
	return slug + ":" + strconv.FormatInt(e.firstSeq(), 10)
}

// url opens the episode's channel. Dispact links a message by its id, which
// the history read does not carry.
func (e episode) url(address, slug string) string {
	return chatlinks.ChannelURL(address, slug)
}

// judged reports whether a message is talk a note may summarise: a bot's own
// posts and the chat's notices about members are not.
func judged(m dispact.Message) bool {
	return !m.IsBot && m.Subtype != "system" && m.Subtype != "member_join"
}

// groupEpisodes cuts messages, oldest first, into episodes. An episode is
// closed once a later message starts the next one, its last message is Quiet
// old, or its day has ended in loc. Only the last episode can be open.
func groupEpisodes(messages []dispact.Message, now time.Time, loc *time.Location) []episode {
	var out []episode
	for _, m := range messages {
		if n := len(out); n > 0 && continues(out[n-1], m, loc) {
			out[n-1].messages = append(out[n-1].messages, m)
			continue
		}
		if n := len(out); n > 0 {
			out[n-1].closed = true
		}
		out = append(out, episode{messages: []dispact.Message{m}})
	}
	if n := len(out); n > 0 {
		last := out[n-1]
		end := last.messages[len(last.messages)-1].PostedAt
		out[n-1].closed = now.Sub(end) >= Quiet || !sameDay(last.messages[0].PostedAt, now, loc)
	}
	return out
}

// continues reports whether m belongs to the episode e: posted within Quiet
// of e's last message, on e's day.
func continues(e episode, m dispact.Message, loc *time.Location) bool {
	last := e.messages[len(e.messages)-1].PostedAt
	return m.PostedAt.Sub(last) < Quiet && sameDay(e.messages[0].PostedAt, m.PostedAt, loc)
}

func sameDay(a, b time.Time, loc *time.Location) bool {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}
