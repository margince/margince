// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type wireParty struct {
	ID idString `json:"id"`
}

type wireAttachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL         string   `json:"url"`
		Thumbnail   string   `json:"thumbnail"`
		Name        string   `json:"name"`
		Size        idString `json:"size"`
		Checksum    string   `json:"checksum"`
		Type        string   `json:"type"`
		UserID      idString `json:"user_id"`
		Phone       string   `json:"phone"`
		Coordinates *struct {
			Latitude  idString `json:"latitude"`
			Longitude idString `json:"longitude"`
		} `json:"coordinates"`
	} `json:"payload"`
}

type wireEvent struct {
	AppID       idString  `json:"app_id"`
	UserIDByApp idString  `json:"user_id_by_app"`
	OAID        idString  `json:"oa_id"`
	EventName   string    `json:"event_name"`
	Timestamp   idString  `json:"timestamp"`
	Sender      wireParty `json:"sender"`
	Recipient   wireParty `json:"recipient"`
	Follower    wireParty `json:"follower"`
	Message     struct {
		MsgID       idString         `json:"msg_id"`
		MsgIDs      []idString       `json:"msg_ids"`
		QuoteMsgID  idString         `json:"quote_msg_id"`
		Text        string           `json:"text"`
		Attachments []wireAttachment `json:"attachments"`
	} `json:"message"`
}

var messageTypes = map[string]MessageType{
	"text": MessageText, "image": MessageImage, "file": MessageFile,
	"sticker": MessageSticker, "gif": MessageGIF, "link": MessageLink,
	"audio": MessageAudio, "video": MessageVideo, "location": MessageLocation,
	"business_card": MessageBusinessCard, "list": MessageList,
}

var receiptEvents = map[string]bool{"user_seen_message": true, "user_received_message": true}

var reactionEvents = map[string]bool{"user_reacted_message": true, "oa_reacted_message": true}

// Decode turns a verified webhook body into a typed Event. Malformed input
// returns ErrMalformedEvent.
func Decode(body []byte) (Event, error) {
	var w wireEvent
	if err := json.Unmarshal(body, &w); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrMalformedEvent, err)
	}
	if w.EventName == "" {
		return Event{}, fmt.Errorf("%w: event_name is missing", ErrMalformedEvent)
	}
	ms, err := strconv.ParseInt(string(w.Timestamp), 10, 64)
	if err != nil || ms <= 0 {
		return Event{}, fmt.Errorf("%w: timestamp is missing or not milliseconds", ErrMalformedEvent)
	}
	ev := Event{
		Name:        w.EventName,
		AppID:       string(w.AppID),
		UserIDByApp: string(w.UserIDByApp),
		OccurredAt:  time.UnixMilli(ms).UTC(),
	}
	switch name := w.EventName; {
	case name == "follow" || name == "unfollow":
		fillFollow(&ev, &w)
	case strings.HasPrefix(name, "user_send_"):
		fillMessage(&ev, &w, Inbound, strings.TrimPrefix(name, "user_send_"))
	case strings.HasPrefix(name, "oa_send_"):
		fillMessage(&ev, &w, Outbound, strings.TrimPrefix(name, "oa_send_"))
	case receiptEvents[name] || reactionEvents[name]:
		ev.Kind = KindReceipt
		if reactionEvents[name] {
			ev.Kind = KindReaction
		}
		fillParties(&ev, &w, strings.HasPrefix(name, "user_"))
		ev.MessageIDs = messageIDs(&w)
	default:
		ev.Kind = KindOther
		ev.OAID = string(w.OAID)
		return ev, nil
	}
	if ev.UserID == "" || ev.OAID == "" {
		return Event{}, fmt.Errorf("%w: %s names no customer or no Official Account", ErrMalformedEvent, w.EventName)
	}
	return ev, nil
}

// fillParties assigns the customer and the OA from sender and recipient: the
// customer is the sender when the customer acted. An explicit oa_id wins over
// the position.
func fillParties(ev *Event, w *wireEvent, customerActed bool) {
	user, oa := w.Recipient.ID, w.Sender.ID
	if customerActed {
		user, oa = w.Sender.ID, w.Recipient.ID
	}
	ev.UserID = string(user)
	ev.OAID = firstNonEmpty(string(w.OAID), string(oa))
}

func fillMessage(ev *Event, w *wireEvent, dir Direction, suffix string) {
	ev.Kind = KindMessage
	fillParties(ev, w, dir == Inbound)
	typ, known := messageTypes[suffix]
	if !known {
		typ = MessageUnsupported
	}
	msg := &Message{
		ID:        string(w.Message.MsgID),
		Direction: dir,
		Type:      typ,
		Text:      w.Message.Text,
		ReplyToID: string(w.Message.QuoteMsgID),
	}
	for _, a := range w.Message.Attachments {
		msg.Attachments = append(msg.Attachments, a.decode())
	}
	ev.Message = msg
}

func (a wireAttachment) decode() Attachment {
	out := Attachment{
		Type:         a.Type,
		URL:          a.Payload.URL,
		ThumbnailURL: a.Payload.Thumbnail,
		Name:         a.Payload.Name,
		Checksum:     a.Payload.Checksum,
		FileType:     a.Payload.Type,
		UserID:       string(a.Payload.UserID),
		Phone:        a.Payload.Phone,
	}
	if size, err := strconv.ParseInt(string(a.Payload.Size), 10, 64); err == nil {
		out.Size = size
	}
	if c := a.Payload.Coordinates; c != nil {
		lat, latErr := strconv.ParseFloat(string(c.Latitude), 64)
		lon, lonErr := strconv.ParseFloat(string(c.Longitude), 64)
		if latErr == nil && lonErr == nil {
			out.Coordinates = &Coordinates{Latitude: lat, Longitude: lon}
		}
	}
	return out
}

func messageIDs(w *wireEvent) []string {
	var ids []string
	if w.Message.MsgID != "" {
		ids = append(ids, string(w.Message.MsgID))
	}
	for _, id := range w.Message.MsgIDs {
		ids = append(ids, string(id))
	}
	return ids
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func fillFollow(ev *Event, w *wireEvent) {
	ev.Kind = KindFollow
	if w.EventName == "unfollow" {
		ev.Kind = KindUnfollow
	}
	ev.UserID = string(w.Follower.ID)
	ev.OAID = firstNonEmpty(string(w.OAID), string(w.Recipient.ID))
}
