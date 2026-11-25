// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"bytes"
	"encoding/json"
)

// wireTime is Meta's unix-seconds timestamp, which arrives as a string on
// some fields and an integer on others.
type wireTime string

func (t *wireTime) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	*t = wireTime(bytes.Trim(b, `"`))
	return nil
}

// The JSON shapes of Meta's webhook payload. They stay unexported: decode.go
// turns them into Message and Status, so no wire detail leaks to a caller.

const (
	typeText      = "text"
	fieldMessages = "messages"
	fieldEchoes   = "smb_message_echoes"
	objectWABA    = "whatsapp_business_account"
)

type wirePayload struct {
	Object string            `json:"object"`
	Entry  []json.RawMessage `json:"entry"`
}

type wireEntry struct {
	ID      string            `json:"id"`
	Changes []json.RawMessage `json:"changes"`
}

type wireChange struct {
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

type wireMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type wireValue struct {
	Metadata      wireMetadata      `json:"metadata"`
	Contacts      []wireContact     `json:"contacts"`
	Messages      []json.RawMessage `json:"messages"`
	Statuses      []json.RawMessage `json:"statuses"`
	MessageEchoes []json.RawMessage `json:"message_echoes"`
}

type wireContact struct {
	WaID    string `json:"wa_id"`
	UserID  string `json:"user_id"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

type wireMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
}

type wireError struct {
	Code int `json:"code"`
}

// wireBody is the part of a message named after its type. It is embedded in a
// message and in an edit, whose content has the same shape.
type wireBody struct {
	Text *struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *wireMedia `json:"image"`
	Video    *wireMedia `json:"video"`
	Audio    *wireMedia `json:"audio"`
	Document *wireMedia `json:"document"`
	Sticker  *wireMedia `json:"sticker"`
	Reaction *struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	} `json:"reaction"`
	Button *struct {
		Text string `json:"text"`
	} `json:"button"`
	Interactive *struct {
		ButtonReply *struct {
			Title string `json:"title"`
		} `json:"button_reply"`
		ListReply *struct {
			Title string `json:"title"`
		} `json:"list_reply"`
	} `json:"interactive"`
}

// wireRef is the content of an `edit` or `revoke` message: a reference to the
// original message and, for an edit, the replacement content either directly or
// under `message`. The field names Meta uses for the reference are the part of
// this payload least confirmed, so they are all listed here and nowhere else.
type wireRef struct {
	OriginalMessageID string `json:"original_message_id"`
	MessageID         string `json:"message_id"`
	Message           *struct {
		Type string `json:"type"`
		wireBody
	} `json:"message"`
	wireBody
}

func (r wireRef) originalID() string {
	if r.OriginalMessageID != "" {
		return r.OriginalMessageID
	}
	return r.MessageID
}

// wireMessage covers both `messages` items and `message_echoes` items; an echo
// differs in carrying `to` and `to_user_id` instead of `from`.
type wireMessage struct {
	ID             string   `json:"id"`
	From           string   `json:"from"`
	FromUserID     string   `json:"from_user_id"`
	To             string   `json:"to"`
	ToUserID       string   `json:"to_user_id"`
	ToParentUserID string   `json:"to_parent_user_id"`
	Timestamp      wireTime `json:"timestamp"`
	Type           string   `json:"type"`
	wireBody
	Context *struct {
		Forwarded           bool   `json:"forwarded"`
		FrequentlyForwarded bool   `json:"frequently_forwarded"`
		ID                  string `json:"id"`
	} `json:"context"`
	Edit   *wireRef    `json:"edit"`
	Revoke *wireRef    `json:"revoke"`
	Errors []wireError `json:"errors"`
}

type wireStatus struct {
	ID          string      `json:"id"`
	Status      string      `json:"status"`
	Timestamp   wireTime    `json:"timestamp"`
	RecipientID string      `json:"recipient_id"`
	Errors      []wireError `json:"errors"`
}
