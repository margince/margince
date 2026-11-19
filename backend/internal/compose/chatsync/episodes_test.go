// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
)

var day = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func msg(seq int64, at time.Time) dispact.Message {
	return dispact.Message{TS: "0." + fmt.Sprintf("%06d", seq), Seq: seq, PostedAt: at, Text: "talk", UserName: "Rita"}
}

func seqsOf(e episode) []int64 {
	out := make([]int64, 0, len(e.messages))
	for _, m := range e.messages {
		out = append(out, m.Seq)
	}
	return out
}

func TestAThreadAnsweredWithinTheQuietWindowIsOneEpisode(t *testing.T) {
	root := msg(1, day)
	reply1 := msg(2, day.Add(20*time.Minute))
	reply1.ThreadTS = root.TS
	reply2 := msg(3, day.Add(45*time.Minute))
	reply2.ThreadTS = root.TS
	now := day.Add(2 * time.Hour)

	episodes := groupEpisodes([]dispact.Message{root, reply1, reply2}, now, time.UTC)

	if len(episodes) != 1 || !episodes[0].closed {
		t.Fatalf("episodes = %d, want one closed episode for the thread", len(episodes))
	}
	if got := seqsOf(episodes[0]); len(got) != 3 || episodes[0].key("acme") != "acme:1" {
		t.Fatalf("episode = %v keyed %q, want seqs 1..3 keyed acme:1", got, episodes[0].key("acme"))
	}
}

func TestThirtyMinutesOfQuietCloseAnEpisode(t *testing.T) {
	messages := []dispact.Message{msg(1, day), msg(2, day.Add(10*time.Minute)), msg(3, day.Add(40*time.Minute))}
	now := day.Add(50 * time.Minute)

	episodes := groupEpisodes(messages, now, time.UTC)

	if len(episodes) != 2 {
		t.Fatalf("episodes = %d, want two split by the quiet half hour", len(episodes))
	}
	if !episodes[0].closed || episodes[0].lastSeq() != 2 {
		t.Errorf("first episode closed=%t last=%d, want closed at seq 2", episodes[0].closed, episodes[0].lastSeq())
	}
	if episodes[1].closed {
		t.Error("the second episode is ten minutes old and must stay open")
	}
}

func TestTheEndOfTheDayClosesAnEpisode(t *testing.T) {
	late := time.Date(2026, 11, 10, 23, 50, 0, 0, time.UTC)
	messages := []dispact.Message{msg(1, late), msg(2, late.Add(15*time.Minute))}

	episodes := groupEpisodes(messages, late.Add(20*time.Minute), time.UTC)

	if len(episodes) != 2 || !episodes[0].closed {
		t.Fatalf("episodes = %d, want the midnight to split them and close the first", len(episodes))
	}
	if episodes[1].closed {
		t.Error("the episode after midnight is five minutes old and must stay open")
	}

	quietDay := groupEpisodes([]dispact.Message{msg(1, late)}, late.Add(15*time.Minute), time.UTC)
	if !quietDay[0].closed {
		t.Error("an episode whose day ended is closed even before its quiet half hour")
	}
}

func TestBotPostsAndTheChatsNoticesAreNotJudged(t *testing.T) {
	bot := msg(1, day)
	bot.IsBot = true
	if judged(bot) {
		t.Error("a bot's post was judged")
	}
	for _, subtype := range []string{
		"system", "member_join", "member_added", "member_removed", "member_left", "channel_join", "channel_leave",
		"channel_name", "channel_topic", "channel_purpose", "channel_archive", "channel_unarchive", "call_invite",
	} {
		notice := msg(2, day)
		notice.Subtype = subtype
		if judged(notice) {
			t.Errorf("a %q line was judged as talk", subtype)
		}
	}
}

func TestEveryMessageAPersonWroteIsJudgedWhateverItsLabel(t *testing.T) {
	for _, subtype := range []string{"", "thread_broadcast", "file_share", "me_message", "a_label_the_chat_adds_later"} {
		said := msg(3, day)
		said.Subtype = subtype
		if !judged(said) {
			t.Errorf("a message labelled %q was not judged", subtype)
		}
	}
}
