// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrMalformedPayload marks a body that is not a WhatsApp Business Account
// webhook at all. Items that are bad inside an otherwise valid body are
// reported per item in Result.Errors instead.
var ErrMalformedPayload = errors.New("whatsapp: malformed webhook payload")

// ErrMalformedItem is the cause carried by every ItemError.
var ErrMalformedItem = errors.New("whatsapp: malformed webhook item")

// Decode turns one webhook POST body (already signature-verified) into typed
// events. Only a body that is not a whatsapp_business_account payload fails
// outright. A bad entry, change or message goes to Result.Errors and the
// remaining items still decode. A field this layer does not know is listed in
// UnhandledFields, not failed, so Meta does not retry it.
func Decode(body []byte) (Result, error) {
	var payload wirePayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Object != objectWABA {
		return Result{}, ErrMalformedPayload
	}
	var res Result
	for i, raw := range payload.Entry {
		path := fmt.Sprintf("entry[%d]", i)
		var entry wireEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			res.itemError(path)
			continue
		}
		for j, rawChange := range entry.Changes {
			res.decodeChange(fmt.Sprintf("%s.changes[%d]", path, j), entry.ID, rawChange)
		}
	}
	return res, nil
}

func (r *Result) itemError(path string) {
	r.Errors = append(r.Errors, ItemError{Path: path, Err: ErrMalformedItem})
}

func (r *Result) decodeChange(path, wabaID string, raw json.RawMessage) {
	var change wireChange
	if err := json.Unmarshal(raw, &change); err != nil {
		r.itemError(path)
		return
	}
	if change.Field != fieldMessages && change.Field != fieldEchoes {
		r.UnhandledFields = append(r.UnhandledFields, change.Field)
		return
	}
	var value wireValue
	if err := json.Unmarshal(change.Value, &value); err != nil {
		r.itemError(path + ".value")
		return
	}
	scope := messageScope{wabaID: wabaID, meta: value.Metadata, contacts: value.Contacts}
	if change.Field == fieldEchoes {
		for k, rawMsg := range value.MessageEchoes {
			r.addMessage(fmt.Sprintf("%s.value.message_echoes[%d]", path, k), scope, Outbound, rawMsg)
		}
		return
	}
	for k, rawMsg := range value.Messages {
		r.addMessage(fmt.Sprintf("%s.value.messages[%d]", path, k), scope, Inbound, rawMsg)
	}
	for k, rawStatus := range value.Statuses {
		spath := fmt.Sprintf("%s.value.statuses[%d]", path, k)
		status, err := decodeStatus(value.Metadata.PhoneNumberID, rawStatus)
		if err != nil {
			r.itemError(spath)
			continue
		}
		r.Statuses = append(r.Statuses, status)
	}
}

// messageScope is what a change's value says about every message in it.
type messageScope struct {
	wabaID   string
	meta     wireMetadata
	contacts []wireContact
}

// contact finds the contacts[] entry for a customer by phone or user id.
func (s messageScope) contact(phone, userID string) (wireContact, bool) {
	for _, c := range s.contacts {
		if (phone != "" && normalizePhone(c.WaID) == phone) || (userID != "" && c.UserID == userID) {
			return c, true
		}
	}
	return wireContact{}, false
}

func normalizePhone(p string) string { return strings.TrimPrefix(p, "+") }

func (r *Result) addMessage(path string, scope messageScope, dir Direction, raw json.RawMessage) {
	msg, err := decodeMessage(scope, dir, raw)
	if err != nil {
		r.itemError(path)
		return
	}
	r.Messages = append(r.Messages, msg)
}

func decodeMessage(scope messageScope, dir Direction, raw json.RawMessage) (Message, error) {
	var w wireMessage
	if err := json.Unmarshal(raw, &w); err != nil {
		return Message{}, err
	}
	at, err := parseUnix(string(w.Timestamp))
	if err != nil || w.ID == "" {
		return Message{}, ErrMalformedItem
	}
	phone, userID := normalizePhone(w.From), w.FromUserID
	if dir == Outbound {
		phone, userID = normalizePhone(w.To), w.ToUserID
	}
	contact, found := scope.contact(phone, userID)
	if found {
		if phone == "" {
			phone = normalizePhone(contact.WaID)
		}
		if userID == "" {
			userID = contact.UserID
		}
	}
	if (phone == "" && userID == "") || scope.meta.PhoneNumberID == "" {
		return Message{}, ErrMalformedItem
	}
	msg := Message{
		WABAID:             scope.wabaID,
		PhoneNumberID:      scope.meta.PhoneNumberID,
		DisplayPhone:       scope.meta.DisplayPhoneNumber,
		ID:                 w.ID,
		Direction:          dir,
		CounterpartyPhone:  phone,
		CounterpartyUserID: userID,
		DisplayName:        contact.Profile.Name,
		At:                 at,
		Type:               w.Type,
		Kind:               KindPlaceholder,
	}
	if w.Context != nil {
		msg.Forwarded = w.Context.Forwarded || w.Context.FrequentlyForwarded
		msg.FrequentlyForwarded = w.Context.FrequentlyForwarded
		msg.ReplyToID = w.Context.ID
	}
	for _, e := range w.Errors {
		msg.ErrorCodes = append(msg.ErrorCodes, e.Code)
	}
	w.fillContent(&msg)
	return msg, nil
}

// fillContent sets the content fields from whichever body the message type
// carries. A type with no body this layer reads stays a placeholder.
func (w wireMessage) fillContent(msg *Message) {
	switch {
	case w.Type == "edit" && w.Edit != nil:
		msg.OriginalID = w.Edit.originalID()
		body, typ := w.Edit.wireBody, ""
		if w.Edit.Message != nil {
			body, typ = w.Edit.Message.wireBody, w.Edit.Message.Type
		}
		body.fill(msg, typ)
		msg.Kind = KindEdit
	case w.Type == "revoke" && w.Revoke != nil:
		msg.OriginalID, msg.Kind = w.Revoke.originalID(), KindRevoke
	default:
		w.fill(msg, w.Type)
	}
}

// fill reads the body named by typ; an empty typ takes the first body present,
// for an edit that states no type of its own.
func (b wireBody) fill(msg *Message, typ string) {
	media := map[string]*wireMedia{
		"image": b.Image, "video": b.Video, "audio": b.Audio, "document": b.Document, "sticker": b.Sticker,
	}
	if typ == "" {
		for _, name := range []string{"text", "image", "video", "audio", "document", "sticker"} {
			if media[name] != nil || (name == "text" && b.Text != nil) {
				typ = name
				break
			}
		}
	}
	switch {
	case typ == "text" && b.Text != nil:
		msg.Kind, msg.Text = KindText, b.Text.Body
	case media[typ] != nil:
		m := media[typ]
		msg.Kind = KindMedia
		msg.Media = &Media{Type: typ, ID: m.ID, URL: m.URL, MimeType: m.MimeType, SHA256: m.SHA256, Caption: m.Caption, Filename: m.Filename}
		msg.Text = m.Caption
	case typ == "reaction" && b.Reaction != nil:
		msg.Kind, msg.Text, msg.ReplyToID = KindReaction, b.Reaction.Emoji, b.Reaction.MessageID
	case typ == "button" && b.Button != nil:
		msg.Kind, msg.Text = KindText, b.Button.Text
	case typ == "interactive" && b.Interactive != nil:
		switch {
		case b.Interactive.ButtonReply != nil:
			msg.Kind, msg.Text = KindText, b.Interactive.ButtonReply.Title
		case b.Interactive.ListReply != nil:
			msg.Kind, msg.Text = KindText, b.Interactive.ListReply.Title
		}
	}
}

func decodeStatus(phoneNumberID string, raw json.RawMessage) (Status, error) {
	var w wireStatus
	if err := json.Unmarshal(raw, &w); err != nil {
		return Status{}, err
	}
	at, err := parseUnix(string(w.Timestamp))
	if err != nil || w.ID == "" || w.Status == "" {
		return Status{}, ErrMalformedItem
	}
	st := Status{PhoneNumberID: phoneNumberID, MessageID: w.ID, State: w.Status, RecipientID: w.RecipientID, At: at}
	for _, e := range w.Errors {
		st.ErrorCodes = append(st.ErrorCodes, e.Code)
	}
	return st, nil
}

// parseUnix reads Meta's timestamp, a string of unix seconds.
func parseUnix(s string) (time.Time, error) {
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{}, ErrMalformedItem
	}
	return time.Unix(secs, 0).UTC(), nil
}
