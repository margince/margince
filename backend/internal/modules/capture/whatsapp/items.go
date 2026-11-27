// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"encoding/json"
	"fmt"
)

// Item is one message, echo or delivery status of a webhook POST, cut out as a
// webhook body of its own. Decode reads Body again to the one event it holds, so
// a stored item needs no second decoder.
type Item struct {
	// PhoneNumberID is the business number the item belongs to, which decides the
	// connection it is stored under.
	PhoneNumberID string
	// Key is the provider's redelivery key for the item. The same item sent again
	// has the same Key. A status for another state of one message does not.
	Key string
	// Identities name the customer by CustomerIdentities, or hold a status's
	// recipient. It is empty when the item names no customer.
	Identities []Identity
	// WABAID is the WhatsApp Business Account the entry names.
	WABAID string
	// Status is true for a delivery status, which is no message and files nothing.
	Status bool
	// Body is a whatsapp_business_account payload holding this item and nothing
	// else. Of the batch's contacts it keeps only the customer's own entry.
	Body []byte
}

// Split is what Items cut out of one webhook POST.
type Split struct {
	Items []Item
	// Errors lists the items that could not be decoded; the rest are unaffected.
	Errors []ItemError
	// UnhandledFields lists change fields this layer does not decode.
	UnhandledFields []string
}

// SplitItems cuts a signature-verified webhook body into one Item per message,
// echo and status. Only a body that is not a whatsapp_business_account payload
// fails outright; a bad item goes to Errors and the others still come out.
func SplitItems(body []byte) (Split, error) {
	var payload wirePayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Object != objectWABA {
		return Split{}, ErrMalformedPayload
	}
	var out Split
	for i, raw := range payload.Entry {
		path := fmt.Sprintf("entry[%d]", i)
		var entry wireEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			out.Errors = append(out.Errors, ItemError{Path: path, Err: ErrMalformedItem})
			continue
		}
		for j, rawChange := range entry.Changes {
			out.splitChange(fmt.Sprintf("%s.changes[%d]", path, j), entry.ID, rawChange)
		}
	}
	return out, nil
}

// itemValue is the change value of a single-item body. Metadata stays raw so
// the stored item keeps what Meta sent.
type itemValue struct {
	Metadata      json.RawMessage   `json:"metadata"`
	Contacts      []json.RawMessage `json:"contacts,omitempty"`
	Messages      []json.RawMessage `json:"messages,omitempty"`
	MessageEchoes []json.RawMessage `json:"message_echoes,omitempty"`
	Statuses      []json.RawMessage `json:"statuses,omitempty"`
}

type itemBody struct {
	Object string      `json:"object"`
	Entry  []itemEntry `json:"entry"`
}

type itemEntry struct {
	ID      string       `json:"id"`
	Changes []itemChange `json:"changes"`
}

type itemChange struct {
	Field string    `json:"field"`
	Value itemValue `json:"value"`
}

func (s *Split) splitChange(path, wabaID string, raw json.RawMessage) {
	var change wireChange
	if err := json.Unmarshal(raw, &change); err != nil {
		s.itemError(path)
		return
	}
	if change.Field != fieldMessages && change.Field != fieldEchoes {
		s.UnhandledFields = append(s.UnhandledFields, change.Field)
		return
	}
	var value wireValue
	var rawValue struct {
		Metadata json.RawMessage   `json:"metadata"`
		Contacts []json.RawMessage `json:"contacts"`
	}
	if err := json.Unmarshal(change.Value, &value); err != nil {
		s.itemError(path + ".value")
		return
	}
	if err := json.Unmarshal(change.Value, &rawValue); err != nil {
		s.itemError(path + ".value")
		return
	}
	cut := itemCutter{wabaID: wabaID, field: change.Field, metadata: rawValue.Metadata, contacts: rawValue.Contacts, scope: value}
	if change.Field == fieldEchoes {
		for k, item := range value.MessageEchoes {
			s.add(cut.cut(fmt.Sprintf("%s.value.message_echoes[%d]", path, k), itemValue{MessageEchoes: []json.RawMessage{item}}))
		}
		return
	}
	for k, item := range value.Messages {
		s.add(cut.cut(fmt.Sprintf("%s.value.messages[%d]", path, k), itemValue{Messages: []json.RawMessage{item}}))
	}
	for k, item := range value.Statuses {
		s.add(cut.cut(fmt.Sprintf("%s.value.statuses[%d]", path, k), itemValue{Statuses: []json.RawMessage{item}}))
	}
}

func (s *Split) itemError(path string) {
	s.Errors = append(s.Errors, ItemError{Path: path, Err: ErrMalformedItem})
}

func (s *Split) add(item Item, err *ItemError) {
	if err != nil {
		s.Errors = append(s.Errors, *err)
		return
	}
	s.Items = append(s.Items, item)
}

// itemCutter holds what one change says about each of its items.
type itemCutter struct {
	wabaID   string
	field    string
	metadata json.RawMessage
	contacts []json.RawMessage
	scope    wireValue
}

// cut builds the single-item body for value and reads it back through Decode.
// That tells it the item's number and key. Then it drops every contact but the
// customer's.
func (c itemCutter) cut(path string, value itemValue) (Item, *ItemError) {
	value.Metadata = c.metadata
	value.Contacts = c.contacts
	body, err := c.body(value)
	if err != nil {
		return Item{}, &ItemError{Path: path, Err: ErrMalformedItem}
	}
	res, err := Decode(body)
	if err != nil || len(res.Errors) > 0 || len(res.Messages)+len(res.Statuses) != 1 {
		return Item{}, &ItemError{Path: path, Err: ErrMalformedItem}
	}
	item := Item{}
	switch {
	case len(res.Messages) == 1:
		msg := res.Messages[0]
		item.PhoneNumberID = msg.PhoneNumberID
		item.Key = string(msg.Direction) + ":" + msg.ID
		item.Identities = CustomerIdentities(msg)
		item.WABAID = msg.WABAID
		value.Contacts = c.customerContacts(msg)
	default:
		st := res.Statuses[0]
		item.PhoneNumberID = st.PhoneNumberID
		item.Key = "status:" + st.MessageID + ":" + st.State
		item.Identities = []Identity{{UserID: st.RecipientID}}
		item.Status = true
		value.Contacts = nil
	}
	if item.Body, err = c.body(value); err != nil {
		return Item{}, &ItemError{Path: path, Err: ErrMalformedItem}
	}
	return item, nil
}

func (c itemCutter) body(value itemValue) ([]byte, error) {
	return json.Marshal(itemBody{Object: objectWABA, Entry: []itemEntry{
		{ID: c.wabaID, Changes: []itemChange{{Field: c.field, Value: value}}},
	}})
}

// customerContacts keeps the contacts entry the message's decode used, so another
// customer's name and number in the same batch are not stored with it.
func (c itemCutter) customerContacts(msg Message) []json.RawMessage {
	var kept []json.RawMessage
	for i, contact := range c.scope.Contacts {
		own := messageScope{contacts: []wireContact{contact}}
		if _, ok := own.contact(msg.CounterpartyPhone, msg.CounterpartyUserID); ok {
			kept = append(kept, c.contacts[i])
		}
	}
	return kept
}

// RawSourceID is the raw_capture key of one item. The raw store overwrites on a
// repeated key. So the key names the business number as well as the item. The
// same item sent again lands on its own row and on no other number's row.
func RawSourceID(phoneNumberID, key string) string {
	return phoneNumberID + "|" + key
}

// Identity is one way a customer is known: a phone, or a business-scoped user id
// in the scope of its WhatsApp Business Account.
type Identity struct{ Scope, UserID string }

// CustomerIdentity names the customer of a message as a channel identity of
// provider Provider. A customer with a phone is the phone digits in no scope,
// because phone numbers are global. A customer with only a business-scoped user
// id is that id in the scope of the WhatsApp Business Account. The same id under
// another account is a different customer. Both results are empty when the
// message names neither.
func CustomerIdentity(msg Message) (scope, userID string) {
	if all := CustomerIdentities(msg); len(all) > 0 {
		return all[0].Scope, all[0].UserID
	}
	return "", ""
}

// CustomerIdentities lists every identity the message names for its customer, the
// phone first. A message naming a phone and a BSUID names one customer. Both
// identities must reach the contact, so that an erasure suppresses each.
func CustomerIdentities(msg Message) []Identity {
	var out []Identity
	if msg.CounterpartyPhone != "" {
		out = append(out, Identity{UserID: msg.CounterpartyPhone})
	}
	if msg.CounterpartyUserID != "" {
		out = append(out, Identity{Scope: msg.WABAID, UserID: msg.CounterpartyUserID})
	}
	return out
}
