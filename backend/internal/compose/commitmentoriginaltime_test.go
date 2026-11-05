// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestLegacyCommitmentTimeComesFromTheStoredMessage(t *testing.T) {
	original := []byte("From: mira@example.test\r\nTo: ines@example.test\r\nDate: Mon, 7 Sep 2026 15:30:00 +0200\r\nSubject: Pricing\r\n\r\nI will send pricing.\r\n")
	stored, err := capture.EncodeStoredOriginal(original)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 7, 13, 30, 0, 0, time.UTC)
	binaryStored, err := capture.EncodeStoredOriginal(append(original, 0, 0xff))
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{stored, binaryStored} {
		if got := originalMessageTime(payload); !got.Equal(want) {
			t.Fatalf("original message time = %v, want %v", got, want)
		}
	}
}

func TestUnreadableLegacyCommitmentTimeStaysUnknown(t *testing.T) {
	for name, payload := range map[string][]byte{
		"absent":          nil,
		"broken envelope": []byte(`{"encoding":"base64","data":"!"}`),
		"invalid message": []byte("not a mail header\r\n\r\nbody"),
		"missing date":    []byte("From: mira@example.test\r\n\r\nI will send pricing."),
		"invalid date":    []byte("Date: not a date\r\n\r\nI will send pricing."),
	} {
		t.Run(name, func(t *testing.T) {
			if got := originalMessageTime(payload); !got.IsZero() {
				t.Fatalf("unreadable source acquired time %v", got)
			}
		})
	}
}
