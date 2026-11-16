// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// historyWire is a history page as Dispact sends it, in Slack's shape.
type historyWire struct {
	Messages []struct {
		TS         string `json:"ts"`
		ThreadTS   string `json:"thread_ts"`
		ReplyCount int    `json:"reply_count"`
		UserName   string `json:"user_name"`
		UserEmail  string `json:"user_email"`
		Text       string `json:"text"`
		PostedAt   string `json:"posted_at"`
		EditedAt   string `json:"edited_at"`
		Subtype    string `json:"subtype"`
		BotID      string `json:"bot_id"`
	} `json:"messages"`
	ResponseMetadata struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func (w historyWire) page() (HistoryPage, error) {
	out := HistoryPage{Messages: make([]Message, 0, len(w.Messages))}
	for _, m := range w.Messages {
		seq, err := seqOf(m.TS)
		if err != nil {
			return HistoryPage{}, err
		}
		posted, err := timeOf(m.PostedAt)
		if err != nil {
			return HistoryPage{}, err
		}
		edited, err := timeOf(m.EditedAt)
		if err != nil {
			return HistoryPage{}, err
		}
		out.Messages = append(out.Messages, Message{
			TS: m.TS, Seq: seq, ThreadTS: m.ThreadTS, ReplyCount: m.ReplyCount,
			UserName: m.UserName, UserEmail: m.UserEmail, Text: m.Text,
			PostedAt: posted, EditedAt: edited, Subtype: m.Subtype, IsBot: m.BotID != "",
		})
	}
	if next := w.ResponseMetadata.NextCursor; next != "" {
		cursor, err := strconv.ParseInt(next, 10, 64)
		if err != nil {
			return HistoryPage{}, fmt.Errorf("next_cursor %q is not a seq", next)
		}
		out.NextCursor = &cursor
	}
	return out, nil
}

// seqOf decodes Dispact's ts, the seq written as "<seq/1e6>.<seq%1e6>" with
// six digits after the point.
func seqOf(ts string) (int64, error) {
	hi, lo, found := strings.Cut(ts, ".")
	if !found || len(lo) != 6 {
		return 0, fmt.Errorf("ts %q is not a Dispact message ts", ts)
	}
	high, err := strconv.ParseInt(hi, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ts %q is not a Dispact message ts", ts)
	}
	low, err := strconv.ParseInt(lo, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ts %q is not a Dispact message ts", ts)
	}
	return high*1_000_000 + low, nil
}

func timeOf(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
