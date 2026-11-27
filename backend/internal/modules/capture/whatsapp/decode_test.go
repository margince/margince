// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeFixture(t *testing.T, name string) Result {
	t.Helper()
	res, err := Decode(fixture(t, name))
	if err != nil {
		t.Fatalf("Decode(%s): %v", name, err)
	}
	return res
}

func TestDecodeInboundText(t *testing.T) {
	res := decodeFixture(t, "inbound_text.json")
	want := Message{
		WABAID: "WABA1", PhoneNumberID: "PNID1", DisplayPhone: "15550001111", ID: "wamid.IN1",
		Direction: Inbound, CounterpartyPhone: "491701234567", DisplayName: "Ada Customer",
		At: time.Unix(1764000000, 0).UTC(), Kind: KindText, Type: "text", Text: "Hello, is the offer still valid?",
	}
	if len(res.Messages) != 1 || !reflect.DeepEqual(res.Messages[0], want) {
		t.Fatalf("messages = %+v, want [%+v]", res.Messages, want)
	}
}

func TestDecodeInboundMedia(t *testing.T) {
	res := decodeFixture(t, "inbound_media.json")
	if len(res.Messages) != 5 || len(res.Errors) != 0 {
		t.Fatalf("got %d messages, %v errors", len(res.Messages), res.Errors)
	}
	img, doc := res.Messages[0], res.Messages[1]
	if img.Kind != KindMedia || img.Media == nil || *img.Media != (Media{Type: "image", ID: "MEDIA1", MimeType: "image/jpeg", SHA256: "abc=", Caption: "the contract"}) {
		t.Errorf("image = %+v / %+v", img, img.Media)
	}
	if img.Text != "the contract" || !img.Forwarded || !img.FrequentlyForwarded {
		t.Errorf("image caption/forward flags = %q %v %v", img.Text, img.Forwarded, img.FrequentlyForwarded)
	}
	if doc.Media == nil || doc.Media.Filename != "offer.pdf" || doc.ReplyToID != "wamid.OUT0" || doc.Forwarded {
		t.Errorf("document = %+v / %+v", doc, doc.Media)
	}
	for i, typ := range []string{"image", "document", "video", "audio", "sticker"} {
		if m := res.Messages[i]; m.Media == nil || m.Media.Type != typ || m.Type != typ {
			t.Errorf("message %d: want media type %s, got %+v", i, typ, m)
		}
	}
}

func TestDecodeOtherTypes(t *testing.T) {
	res := decodeFixture(t, "inbound_other.json")
	type want struct {
		id, text, replyTo string
		kind              Kind
		codes             []int
	}
	wants := []want{
		{"wamid.REACT", "👍", "wamid.OUT0", KindReaction, nil},
		{"wamid.BTN", "Yes", "", KindText, nil},
		{"wamid.INT", "Call me", "", KindText, nil},
		{"wamid.LOC", "", "", KindPlaceholder, nil},
		{"wamid.UNS", "", "", KindPlaceholder, []int{131060}},
		{"wamid.FUT", "", "", KindPlaceholder, nil},
	}
	if len(res.Messages) != len(wants) {
		t.Fatalf("got %d messages, want %d (errors %v)", len(res.Messages), len(wants), res.Errors)
	}
	for i, w := range wants {
		m := res.Messages[i]
		if m.ID != w.id || m.Text != w.text || m.ReplyToID != w.replyTo || m.Kind != w.kind || !reflect.DeepEqual(m.ErrorCodes, w.codes) {
			t.Errorf("message %d = %+v, want %+v", i, m, w)
		}
		if m.Direction != Inbound || m.At.IsZero() || m.CounterpartyPhone != "491701234567" {
			t.Errorf("message %d lost direction, time or counterparty: %+v", i, m)
		}
	}
}

func TestDecodeEchoes(t *testing.T) {
	res := decodeFixture(t, "echo_text.json")
	if len(res.Messages) != 2 || len(res.Statuses) != 0 {
		t.Fatalf("got %d messages %d statuses", len(res.Messages), len(res.Statuses))
	}
	text, img := res.Messages[0], res.Messages[1]
	if text.Direction != Outbound || text.CounterpartyPhone != "491701234567" || text.PhoneNumberID != "PNID1" ||
		text.Text != "Yes, valid until Friday." || text.DisplayName != "Ada Customer" || text.CounterpartyUserID != "BR.ADA" || text.ID != "wamid.ECHO1" {
		t.Errorf("echo text = %+v", text)
	}
	if img.Kind != KindMedia || img.Media == nil || img.Media.ID != "MEDIA9" || img.Direction != Outbound {
		t.Errorf("echo image = %+v", img)
	}
}

func TestDecodeStatuses(t *testing.T) {
	res := decodeFixture(t, "statuses.json")
	want := []Status{
		{PhoneNumberID: "PNID1", MessageID: "wamid.OUT1", State: StatusDelivered, RecipientID: "491701234567", At: time.Unix(1764000200, 0).UTC()},
		{PhoneNumberID: "PNID1", MessageID: "wamid.OUT2", State: StatusFailed, RecipientID: "491701234567", At: time.Unix(1764000201, 0).UTC(), ErrorCodes: []int{131047}},
	}
	if !reflect.DeepEqual(res.Statuses, want) || len(res.Messages) != 0 {
		t.Fatalf("statuses = %+v", res.Statuses)
	}
}

func TestDecodeMixedBatchKeepsGoodItems(t *testing.T) {
	res := decodeFixture(t, "mixed_batch.json")
	var ids []string
	for _, m := range res.Messages {
		ids = append(ids, m.ID+"@"+m.PhoneNumberID+"/"+m.WABAID)
	}
	wantIDs := []string{"wamid.OK1@PNID1/WABA1", "wamid.OK2@PNID1/WABA1", "wamid.ECHO3@PNID2/WABA2"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("messages = %v, want %v", ids, wantIDs)
	}
	wantPaths := []string{
		"entry[0].changes[0].value.messages[1]",
		"entry[0].changes[0].value.messages[2]",
		"entry[0].changes[2].value",
		"entry[1]",
	}
	var paths []string
	for _, e := range res.Errors {
		paths = append(paths, e.Path)
		if !errors.Is(e.Err, ErrMalformedItem) {
			t.Errorf("item error %s cause = %v", e.Path, e.Err)
		}
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Errorf("error paths = %v, want %v", paths, wantPaths)
	}
	wantFields := []string{"account_update", "smb_app_state_sync", "history"}
	if !reflect.DeepEqual(res.UnhandledFields, wantFields) {
		t.Errorf("unhandled = %v, want %v", res.UnhandledFields, wantFields)
	}
}

func TestDecodeUnknownFieldIsNotAnError(t *testing.T) {
	res := decodeFixture(t, "unknown_field.json")
	if !reflect.DeepEqual(res.UnhandledFields, []string{"message_template_status_update"}) || len(res.Errors) != 0 || len(res.Messages) != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestDecodeRejectsAnotherPayload(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      `<html>`,
		"wrong object":  `{"object":"page","entry":[]}`,
		"no object":     `{"entry":[]}`,
		"empty body":    ``,
		"entry not arr": `{"object":"whatsapp_business_account","entry":{}}`,
	} {
		if _, err := Decode([]byte(body)); !errors.Is(err, ErrMalformedPayload) {
			t.Errorf("%s: err = %v, want ErrMalformedPayload", name, err)
		}
	}
}

func TestDecodeMessageNeedsItsIdentity(t *testing.T) {
	const head = `{"object":"whatsapp_business_account","entry":[{"id":"W","changes":[{"field":"messages","value":`
	for name, value := range map[string]string{
		"no phone number id": `{"metadata":{},"messages":[{"from":"1","id":"a","timestamp":"1764000000","type":"text","text":{"body":"x"}}]}`,
		"no message id":      `{"metadata":{"phone_number_id":"P"},"messages":[{"from":"1","timestamp":"1764000000","type":"text","text":{"body":"x"}}]}`,
		"no sender":          `{"metadata":{"phone_number_id":"P"},"messages":[{"id":"a","timestamp":"1764000000","type":"text","text":{"body":"x"}}]}`,
		"zero timestamp":     `{"metadata":{"phone_number_id":"P"},"messages":[{"from":"1","id":"a","timestamp":"0","type":"text","text":{"body":"x"}}]}`,
	} {
		res, err := Decode([]byte(head + value + `}]}]}`))
		if err != nil || len(res.Messages) != 0 || len(res.Errors) != 1 {
			t.Errorf("%s: messages=%d errors=%d err=%v", name, len(res.Messages), len(res.Errors), err)
		}
	}
}

func TestDecodeUsernamesEditsAndRevokes(t *testing.T) {
	res := decodeFixture(t, "username_edit_revoke.json")
	if len(res.Messages) != 5 || len(res.Errors) != 1 || res.Errors[0].Path != "entry[0].changes[0].value.messages[1]" {
		t.Fatalf("messages=%d errors=%v", len(res.Messages), res.Errors)
	}
	byID := map[string]Message{}
	for _, m := range res.Messages {
		byID[m.ID] = m
	}
	if m := byID["wamid.USR1"]; m.CounterpartyPhone != "" || m.CounterpartyUserID != "BR.BOB" || m.DisplayName != "bob_the_builder" {
		t.Errorf("username inbound = %+v", m)
	}
	if m := byID["wamid.E4"]; m.Direction != Outbound || m.CounterpartyPhone != "" || m.CounterpartyUserID != "BR.BOB" || m.At.Unix() != 1764000402 {
		t.Errorf("username echo = %+v", m)
	}
	if m := byID["wamid.E5"]; m.Kind != KindEdit || m.OriginalID != "wamid.E1" || m.Media == nil || m.Media.URL != "https://lookaside.fbsbx.com/x" || m.Text != "fixed caption" {
		t.Errorf("media edit = %+v / %+v", m, m.Media)
	}
	if m := byID["wamid.E6"]; m.Kind != KindEdit || m.OriginalID != "wamid.E2" || m.Text != "corrected text" {
		t.Errorf("text edit = %+v", m)
	}
	if m := byID["wamid.E7"]; m.Kind != KindRevoke || m.OriginalID != "wamid.E1" {
		t.Errorf("revoke = %+v", m)
	}
}
