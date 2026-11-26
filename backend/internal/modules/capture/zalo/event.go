// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package zalo is the protocol layer for a company's Zalo Official Account.
// It verifies webhook signatures and decodes event bodies into typed events.
// It also holds the OA Open API client: token refresh, send text, user detail.
// It files nothing and touches no database; the capture path consumes its types.
package zalo

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Webhook errors. Each is a sentinel a handler maps to a status without
// reading the message.
var (
	// ErrBodyTooLarge reports a webhook body over MaxBodyBytes.
	ErrBodyTooLarge = errors.New("zalo: webhook body exceeds the size limit")
	// ErrSignatureMissing reports a request with no X-ZEvent-Signature header.
	ErrSignatureMissing = errors.New("zalo: webhook signature header is missing")
	// ErrSignatureInvalid reports a header that is malformed or does not match
	// the body.
	ErrSignatureInvalid = errors.New("zalo: webhook signature does not match")
	// ErrMalformedEvent reports a body that is not a decodable Zalo event.
	ErrMalformedEvent = errors.New("zalo: webhook body is not a valid event")
)

// Direction says who wrote a message relative to the Official Account.
type Direction string

// The two directions of a chat message.
const (
	// Inbound is a customer writing to the OA.
	Inbound Direction = "inbound"
	// Outbound is the OA writing to a customer, from OA Admin chat or the API.
	Outbound Direction = "outbound"
)

// Kind classifies an event.
type Kind string

// The event kinds. Everything that is not a chat message is a signal the
// capture path ignores or records without a body.
const (
	KindMessage  Kind = "message"
	KindFollow   Kind = "follow"
	KindUnfollow Kind = "unfollow"
	KindReceipt  Kind = "receipt"
	KindReaction Kind = "reaction"
	// KindOther is an event_name this package does not know and that is not a
	// user_send_* or oa_send_* name.
	KindOther Kind = "other"
)

// MessageType is the content family of a chat message.
type MessageType string

// The message types. MessageUnsupported covers a user_send_* or oa_send_* name
// this package does not know, so the capture path can still show a placeholder.
const (
	MessageText         MessageType = "text"
	MessageImage        MessageType = "image"
	MessageFile         MessageType = "file"
	MessageSticker      MessageType = "sticker"
	MessageGIF          MessageType = "gif"
	MessageLink         MessageType = "link"
	MessageAudio        MessageType = "audio"
	MessageVideo        MessageType = "video"
	MessageLocation     MessageType = "location"
	MessageBusinessCard MessageType = "business_card"
	MessageList         MessageType = "list"
	MessageUnsupported  MessageType = "unsupported"
)

// Attachment is one attachment of a message, with the fields Zalo sends for
// its type. Fields a type does not carry stay zero.
type Attachment struct {
	// Type is Zalo's attachment type, kept as sent: image, file, sticker, gif,
	// link, audio, video, location or a type added later.
	Type         string
	URL          string
	ThumbnailURL string
	Name         string
	Size         int64
	Checksum     string
	// FileType is the payload's own `type` field, for example a file extension.
	FileType string
	// Coordinates is set for a location attachment.
	Coordinates *Coordinates
}

// Coordinates is a location attachment's position.
type Coordinates struct {
	Latitude  float64
	Longitude float64
}

// Message is the chat content of a KindMessage event.
type Message struct {
	// ID is Zalo's msg_id, the provider message id.
	ID          string
	Direction   Direction
	Type        MessageType
	Text        string
	Attachments []Attachment
}

// Event is one decoded webhook delivery.
type Event struct {
	Kind Kind
	// Name is the raw event_name.
	Name  string
	AppID string
	// OAID is the Official Account the event belongs to.
	OAID string
	// UserID is the customer's id in the OA's scope. Empty only for KindOther.
	UserID string
	// UserIDByApp is the customer's id in the app's scope, when Zalo sends it.
	UserIDByApp string
	// OccurredAt is the event's own time, from the body's millisecond timestamp.
	OccurredAt time.Time
	// Message is set for KindMessage.
	Message *Message
	// MessageIDs are the messages a KindReceipt or KindReaction refers to.
	MessageIDs []string
}

// idString decodes a JSON string or number into its text, so an id or
// timestamp that Zalo sends as either decodes the same way.
type idString string

func (s *idString) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "null" {
		return nil
	}
	if strings.HasPrefix(t, `"`) {
		u, err := strconv.Unquote(t)
		if err != nil {
			return err
		}
		*s = idString(u)
		return nil
	}
	if _, err := strconv.ParseFloat(t, 64); err != nil {
		return errors.New("not a string or number")
	}
	*s = idString(t)
	return nil
}
