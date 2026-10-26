// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sendAndParse sends one message through a loopback fake relay and parses
// what the relay received.
func sendAndParse(t *testing.T, subject, body, messageID string) (*mail.Message, string) {
	t.Helper()
	relay := startFakeRelay(t, false)
	host, port := relay.Address(t)
	s := SMTP{Host: host, Port: port, FromAddress: "privacy@example.test"}
	if err := s.SendIdentified(context.Background(), "ada@example.test", subject, body, messageID); err != nil {
		t.Fatalf("SendIdentified: %v", err)
	}
	raw := relay.Received()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("the relay received an unparseable message: %v\n%s", err, raw)
	}
	return msg, raw
}

// decodedBody reads a quoted-printable body back to its text.
func decodedBody(t *testing.T, msg *mail.Message) string {
	t.Helper()
	if got := msg.Header.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
		t.Fatalf("Content-Transfer-Encoding %q, want quoted-printable", got)
	}
	text, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatalf("decoding the body: %v", err)
	}
	return strings.ReplaceAll(string(text), "\r\n", "\n")
}

func TestTheMessageCarriesTheCallersMessageID(t *testing.T) {
	msg, _ := sendAndParse(t, "Reset your password", "body", "confirm-0abc@margince.invalid")
	if got := msg.Header.Get("Message-ID"); got != "<confirm-0abc@margince.invalid>" {
		t.Fatalf("Message-ID %q, want the caller's", got)
	}
}

func TestAPlainSendMintsAUniqueMessageIDOnTheSendingDomain(t *testing.T) {
	first, _ := sendAndParse(t, "Reset your password", "body", "")
	second, _ := sendAndParse(t, "Reset your password", "body", "")
	a, b := first.Header.Get("Message-ID"), second.Header.Get("Message-ID")
	if !strings.HasPrefix(a, "<") || !strings.HasSuffix(a, "@example.test>") {
		t.Fatalf("Message-ID %q, want one on the sender's domain example.test", a)
	}
	local := strings.TrimSuffix(strings.TrimPrefix(a, "<"), "@example.test>")
	if len(local) != 32 || local[12] != '7' {
		t.Fatalf("Message-ID local part %q, want a 32-digit UUIDv7", local)
	}
	if _, err := ids.Parse(local[:8] + "-" + local[8:12] + "-" + local[12:16] + "-" + local[16:20] + "-" + local[20:]); err != nil {
		t.Fatalf("Message-ID local part %q is not a UUID: %v", local, err)
	}
	if a == b {
		t.Fatalf("two messages share the Message-ID %q", a)
	}
}

func TestEveryMessageCarriesADateFromTheClock(t *testing.T) {
	at := time.Date(2026, time.October, 25, 9, 30, 0, 0, time.UTC)
	for name, htmlBody := range map[string]string{"html alternative": "<p>body</p>", "plain text": ""} {
		t.Run(name, func(t *testing.T) {
			relay := startFakeRelay(t, false)
			host, port := relay.Address(t)
			s := SMTP{Host: host, Port: port, FromAddress: "privacy@example.test", Now: func() time.Time { return at }}
			if err := s.SendAlternative(context.Background(), "ada@example.test", "Hello", "body", htmlBody, ""); err != nil {
				t.Fatalf("SendAlternative: %v", err)
			}
			msg, err := mail.ReadMessage(strings.NewReader(relay.Received()))
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}
			got, err := msg.Header.Date()
			if err != nil || !got.Equal(at) {
				t.Fatalf("Date %v (%v), want %v", got, err, at)
			}
		})
	}
}

// A 2,000-rune line would break the 998-octet limit if sent as it is.
func TestALongVietnameseLineIsWrappedAndDecodesBackExactly(t *testing.T) {
	body := strings.Repeat("ữ", 2000) + "\n.dòng bắt đầu bằng dấu chấm\n"
	msg, raw := sendAndParse(t, "s", body, "")

	_, encodedBody, _ := strings.Cut(raw, "\n\n")
	for line := range strings.SplitSeq(encodedBody, "\n") {
		if len(line) > 76 {
			t.Fatalf("a body line is %d octets, want at most 76: %q", len(line), line)
		}
	}
	if got := decodedBody(t, msg); got != body {
		t.Fatalf("the body decoded to a different text (%d runes, want %d)",
			len([]rune(got)), len([]rune(body)))
	}
}

func TestALongVietnameseSubjectIsFoldedAndDecodesBackExactly(t *testing.T) {
	subject := strings.Repeat("ữ", 200)
	msg, raw := sendAndParse(t, subject, "b", "")

	headers, _, _ := strings.Cut(raw, "\n\n")
	for line := range strings.SplitSeq(headers, "\n") {
		if len(line) > headerLineLimit {
			t.Fatalf("a header line is %d octets, want at most %d: %q", len(line), headerLineLimit, line)
		}
	}
	got, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("decoding the subject: %v", err)
	}
	if got != subject {
		t.Fatalf("the subject decoded to %q, want %q", got, subject)
	}
}

func TestAShortSubjectWithSpacesDecodesBackExactly(t *testing.T) {
	subject := "Chúng tôi lưu giữ thông tin gì về quý vị, và từ đâu"
	msg, _ := sendAndParse(t, subject, "b", "")
	got, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || got != subject {
		t.Fatalf("the subject decoded to %q (%v), want %q", got, err, subject)
	}
}

// Once the relay accepted the message, a dropped QUIT must not read as a
// failure: the dispatcher would retry and mail a second copy.
func TestAFailedQuitAfterTheMessageWasAcceptedIsASuccess(t *testing.T) {
	relay := startFakeRelay(t, false)
	relay.DropOnQuit = true
	host, port := relay.Address(t)
	s := SMTP{Host: host, Port: port, FromAddress: "crm@example.test"}

	if err := s.SendIdentified(context.Background(), "ada@example.test", "s", "b", "x@y.test"); err != nil {
		t.Fatalf("SendIdentified reported %v for a message the relay accepted", err)
	}
	if relay.Received() == "" {
		t.Fatal("the relay received nothing, so this test proves nothing about QUIT")
	}
}

func TestAMessageIDThatIsNotAUsableIdentityIsRefused(t *testing.T) {
	s := SMTP{Host: "127.0.0.1", Port: 25, FromAddress: "crm@example.test"}
	for name, id := range map[string]string{
		"whitespace":    "a b@x.test",
		"control":       "a\tb@x.test",
		"line break":    "a@b.test>\r\nBcc: evil@x.test",
		"no at":         "no-at-sign",
		"angle bracket": "a>b@x.test",
		"oversize":      strings.Repeat("a", 510) + "@x.test",
	} {
		err := s.SendIdentified(context.Background(), "ada@example.test", "s", "b", id)
		if !errors.Is(err, connector.ErrInvalidMessageID) {
			t.Errorf("%s: SendIdentified answered %v, want ErrInvalidMessageID", name, err)
		}
	}
}
