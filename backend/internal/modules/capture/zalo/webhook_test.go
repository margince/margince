// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const testSecret = "oa-secret"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func signed(t *testing.T, name string) *http.Request {
	t.Helper()
	body := fixture(t, name)
	var env struct {
		AppID     idString `json:"app_id"`
		Timestamp idString `json:"timestamp"`
	}
	mustUnmarshal(t, body, &env)
	r := httptest.NewRequest(http.MethodPost, "/zalo", bytes.NewReader(body))
	r.Header.Set(SignatureHeader, "mac="+macHex(string(env.AppID), body, string(env.Timestamp), testSecret))
	return r
}

func TestVerifySignatureAgainstIndependentlyComputedFixture(t *testing.T) {
	t.Parallel()
	body := fixture(t, "user_send_text.json")
	// SHA-256 of app_id + body + timestamp + secret, computed outside this package.
	const want = "c4548b091684477b17e058ff8a6cf3675be3d78a55c5271e6f2620b1743b6d4b"
	if err := VerifySignature(testSecret, "mac="+want, body); err != nil {
		t.Fatalf("known-good signature rejected: %v", err)
	}
}

func TestVerifySignatureRefusals(t *testing.T) {
	t.Parallel()
	body := fixture(t, "user_send_text.json")
	good := macHex("1111222233334444", body, "1764070000123", testSecret)
	cases := []struct {
		name, header, secret string
		body                 []byte
		want                 error
	}{
		{"missing", "", testSecret, body, ErrSignatureMissing},
		{"no mac prefix", good, testSecret, body, ErrSignatureInvalid},
		{"not hex", "mac=zz", testSecret, body, ErrSignatureInvalid},
		{"wrong secret", "mac=" + good, "other", body, ErrSignatureInvalid},
		{"empty secret", "mac=" + good, "", body, ErrSignatureInvalid},
		{"tampered body", "mac=" + good, testSecret, bytes.Replace(body, []byte("giá"), []byte("gia"), 1), ErrSignatureInvalid},
		{"unparseable body", "mac=" + good, testSecret, []byte("{"), ErrSignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := VerifySignature(tc.secret, tc.header, tc.body); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestReceiveRejectsOversizedBodyBeforeVerifying(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodPost, "/zalo", strings.NewReader(strings.Repeat("a", MaxBodyBytes+1)))
	if _, err := Receive(r, testSecret); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("got %v, want ErrBodyTooLarge", err)
	}
}

func TestReceiveDecodesASignedEvent(t *testing.T) {
	t.Parallel()
	ev, err := Receive(signed(t, "user_send_text.json"), testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindMessage || ev.Message.Text != "Xin chào, tôi muốn hỏi giá" {
		t.Fatalf("unexpected event %+v", ev)
	}
}

func TestReceiveRefusesAnUnsignedEvent(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodPost, "/zalo", bytes.NewReader(fixture(t, "user_send_text.json")))
	if _, err := Receive(r, testSecret); !errors.Is(err, ErrSignatureMissing) {
		t.Fatalf("got %v, want ErrSignatureMissing", err)
	}
}

func TestDecodeMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file    string
		dir     Direction
		typ     MessageType
		user    string
		oa      string
		msgID   string
		byApp   string
		attachN int
	}{
		{"user_send_text.json", Inbound, MessageText, "5550001", "9990001", "msg-in-1", "7777888899990000", 0},
		{"user_send_image.json", Inbound, MessageImage, "5550001", "9990001", "msg-in-2", "7777888899990000", 1},
		{"user_send_file.json", Inbound, MessageFile, "5550001", "9990001", "msg-in-3", "", 1},
		{"user_send_location.json", Inbound, MessageLocation, "5550001", "9990001", "msg-in-4", "", 1},
		{"user_send_business_card.json", Inbound, MessageBusinessCard, "5550001", "9990001", "msg-in-5", "", 0},
		{"oa_send_text.json", Outbound, MessageText, "5550001", "9990001", "msg-out-1", "7777888899990000", 0},
		{"oa_send_file.json", Outbound, MessageFile, "5550001", "9990001", "msg-out-2", "", 1},
		{"user_send_poll.json", Inbound, MessageUnsupported, "5550001", "9990001", "msg-in-6", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			ev, err := Decode(fixture(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			m := ev.Message
			if ev.Kind != KindMessage || m == nil {
				t.Fatalf("not a message: %+v", ev)
			}
			if m.Direction != tc.dir || m.Type != tc.typ || ev.UserID != tc.user || ev.OAID != tc.oa ||
				m.ID != tc.msgID || ev.UserIDByApp != tc.byApp || len(m.Attachments) != tc.attachN {
				t.Fatalf("got %+v / %+v", ev, m)
			}
			if ev.AppID != "1111222233334444" || ev.OccurredAt.Before(time.UnixMilli(1764070000000)) {
				t.Fatalf("app or time wrong: %+v", ev)
			}
		})
	}
}

func TestDecodeNumericTimestampAndAttachments(t *testing.T) {
	t.Parallel()
	ev, err := Decode(fixture(t, "oa_send_file.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.OccurredAt.Equal(time.UnixMilli(1764070006000)) {
		t.Fatalf("numeric timestamp read as %v", ev.OccurredAt)
	}
	a := ev.Message.Attachments[0]
	if a.Type != "file" || a.Name != "hop-dong.pdf" || a.Size != 1024 || a.Checksum != "def456" || a.FileType != "pdf" || a.URL == "" {
		t.Fatalf("attachment %+v", a)
	}
	img, err := Decode(fixture(t, "user_send_image.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Message.Attachments[0]; got.ThumbnailURL != "https://example.test/t.jpg" || got.URL != "https://example.test/i.jpg" {
		t.Fatalf("image attachment %+v", got)
	}
	loc, err := Decode(fixture(t, "user_send_location.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c := loc.Message.Attachments[0].Coordinates; c == nil || c.Latitude != 10.7769 || c.Longitude != 106.7009 {
		t.Fatalf("coordinates %+v", c)
	}
}

func TestDecodeNonMessageEvents(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file string
		kind Kind
		ids  []string
	}{
		{"follow.json", KindFollow, nil},
		{"unfollow.json", KindUnfollow, nil},
		{"user_seen_message.json", KindReceipt, []string{"msg-out-1", "msg-out-2"}},
		{"oa_reacted_message.json", KindReaction, []string{"msg-in-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			ev, err := Decode(fixture(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if ev.Kind != tc.kind || ev.UserID != "5550001" || ev.OAID != "9990001" || ev.Message != nil {
				t.Fatalf("got %+v", ev)
			}
			if strings.Join(ev.MessageIDs, ",") != strings.Join(tc.ids, ",") {
				t.Fatalf("message ids %v, want %v", ev.MessageIDs, tc.ids)
			}
		})
	}
}

func TestDecodeUnknownEventNameIsKindOther(t *testing.T) {
	t.Parallel()
	ev, err := Decode(fixture(t, "user_submit_info.json"))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindOther || ev.Name != "user_submit_info" || ev.Message != nil {
		t.Fatalf("got %+v", ev)
	}
}

func TestDecodeOAIDFieldWinsOverPosition(t *testing.T) {
	t.Parallel()
	body := `{"app_id":"a","oa_id":"explicit","event_name":"user_send_text","timestamp":"1764070000000","sender":{"id":"u"},"recipient":{"id":"positional"},"message":{"msg_id":"m","text":"x"}}`
	ev, err := Decode([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if ev.OAID != "explicit" || ev.UserID != "u" {
		t.Fatalf("got %+v", ev)
	}
}

func TestDecodeMalformedInput(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty":              ``,
		"not json":           `<html>`,
		"array":              `[]`,
		"no event name":      `{"timestamp":"1764070000000"}`,
		"no timestamp":       `{"event_name":"user_send_text","sender":{"id":"u"},"recipient":{"id":"o"}}`,
		"bad timestamp":      `{"event_name":"user_send_text","timestamp":"soon","sender":{"id":"u"},"recipient":{"id":"o"}}`,
		"no customer":        `{"event_name":"user_send_text","timestamp":"1764070000000","recipient":{"id":"o"}}`,
		"no oa":              `{"event_name":"oa_send_text","timestamp":"1764070000000","recipient":{"id":"u"}}`,
		"object as id":       `{"event_name":"user_send_text","timestamp":"1764070000000","sender":{"id":{}},"recipient":{"id":"o"}}`,
		"follow no follower": `{"event_name":"follow","timestamp":"1764070000000","oa_id":"o"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode([]byte(body)); !errors.Is(err, ErrMalformedEvent) {
				t.Fatalf("got %v, want ErrMalformedEvent", err)
			}
		})
	}
}
