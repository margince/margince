// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SlackReceiver accepts signed Slack changes as cues for canonical source reads.
type SlackReceiver struct {
	*Receiver
	Roots RootNotices
}

type slackNotice struct {
	Type      string `json:"type"`
	ID        string `json:"event_id"`
	Team      string `json:"team_id"`
	Challenge string `json:"challenge"`
	Event     struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Root    string `json:"thread_ts"`
		Deleted string `json:"deleted_ts"`
		Subtype string `json:"subtype"`
		Message struct {
			TS   string `json:"ts"`
			Root string `json:"thread_ts"`
		} `json:"message"`
		Previous struct {
			TS   string `json:"ts"`
			Root string `json:"thread_ts"`
		} `json:"previous_message"`
	} `json:"event"`
}

// Receive validates provider notifications before queueing source synchronization.
func (s *SlackReceiver) Receive(ctx context.Context, d chatlinks.Delivery) (crmcontracts.DispactChannelEventReceipt, error) {
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	var none crmcontracts.DispactChannelEventReceipt
	bot, secret, err := s.Bots.LiveBotEvents(ctx, chatlinks.ProviderSlack)
	if err != nil {
		return none, chatlinks.ErrEventRejected
	}
	if err := verifySlackNotice(secret, d, s.Now()); err != nil {
		return none, err
	}
	var notice slackNotice
	if json.Unmarshal(d.Body, &notice) != nil {
		return none, chatlinks.ErrEventMalformed
	}
	if slackChallenge(notice) {
		return crmcontracts.DispactChannelEventReceipt{Challenge: &notice.Challenge}, nil
	}
	if !validSlackEvent(notice, bot.TeamID) {
		return none, chatlinks.ErrEventRejected
	}
	key := "slack-notice|" + bot.ConnectionID.String() + "|" + notice.ID
	if err := s.spend(ctx, key); errors.Is(err, chatlinks.ErrEventReplayed) {
		return crmcontracts.DispactChannelEventReceipt{Status: new(crmcontracts.DispactChannelEventReceiptStatusAlreadyTaken)}, nil
	} else if err != nil {
		return none, err
	}
	if notice.Event.Channel == "" {
		return crmcontracts.DispactChannelEventReceipt{Status: new(crmcontracts.DispactChannelEventReceiptStatusIgnored)}, nil
	}
	root := slackNoticeRoot(notice)
	if err := s.Roots.InvalidateNativeRoot(ctx, bot.ConnectionID, notice.Event.Channel, chatwire.RootRef{ID: root, Revision: notice.ID, Deleted: notice.Event.Subtype == "message_deleted" && notice.Event.Deleted == root}); err != nil {
		s.release(ctx, key)
		return none, err
	}
	if err := s.Pull(ctx, bot.ConnectionID); err != nil {
		s.release(ctx, key)
		return none, err
	}
	if s.Marks != nil {
		if err := s.Marks.Mark(ctx, noticeKey(bot.ConnectionID), s.Now(), PushFresh); err != nil {
			s.release(ctx, key)
			return none, err
		}
	}
	return crmcontracts.DispactChannelEventReceipt{Status: new(crmcontracts.DispactChannelEventReceiptStatusPullQueued)}, nil
}

func slackNoticeRoot(notice slackNotice) string {
	event := notice.Event
	for _, root := range []string{event.Root, event.Message.Root, event.Previous.Root, event.Message.TS, event.Previous.TS, event.Deleted, event.TS} {
		if root != "" {
			return root
		}
	}
	return ""
}

func verifySlackNotice(secret string, d chatlinks.Delivery, now time.Time) error {
	seconds, err := strconv.ParseInt(d.Timestamp, 10, 64)
	if err != nil || now.Sub(time.Unix(seconds, 0)) > noticeSkew || time.Unix(seconds, 0).Sub(now) > noticeSkew {
		return chatlinks.ErrEventRejected
	}
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte("v0:" + d.Timestamp + ":")); err != nil {
		return chatlinks.ErrEventRejected
	}
	if _, err := mac.Write(d.Body); err != nil {
		return chatlinks.ErrEventRejected
	}
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(d.Signature)) {
		return chatlinks.ErrEventRejected
	}
	return nil
}

func validSlackEvent(notice slackNotice, team string) bool {
	return notice.Type == "event_callback" && notice.ID != "" && len(notice.ID) <= 128 && notice.Team == team
}

func slackChallenge(notice slackNotice) bool {
	return notice.Type == "url_verification" && notice.Challenge != ""
}
