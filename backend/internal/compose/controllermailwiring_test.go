// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/comms"
)

// identifiedRecorder is a relay that can carry a Message-ID, and records the
// one it was handed.
type identifiedRecorder struct{ messageID *string }

func (identifiedRecorder) Send(context.Context, string, string, string) error { return nil }

func (r identifiedRecorder) SendIdentified(_ context.Context, _, _, _, messageID string) error {
	*r.messageID = messageID
	return nil
}

// A bounce of the installation's own mail is matched by the delivery row's
// Message-ID. So the relay must put that identity on the wire.
func TestTheControllerRelaySendsUnderTheDeliveryRowsMessageID(t *testing.T) {
	var sent string
	relay := ControllerRelayFor(identifiedRecorder{messageID: &sent})

	err := relay.SendControllerMail(context.Background(), comms.ControllerMessage{
		To: "ada@example.test", Subject: "s", TextBody: "b",
		MessageID: "confirm-0abc@margince.invalid",
	})
	if err != nil {
		t.Fatalf("SendControllerMail: %v", err)
	}
	if sent != "confirm-0abc@margince.invalid" {
		t.Fatalf("relay sent under message id %q, want the delivery row's", sent)
	}
}

func TestControllerMarkupCannotFallBackToAnUnidentifiedPlainSend(t *testing.T) {
	var sent string
	relay := ControllerRelayFor(identifiedRecorder{messageID: &sent})
	err := relay.SendControllerMail(context.Background(), comms.ControllerMessage{To: "reader@example.test", Subject: "Confirm", TextBody: "Body", HTMLBody: "<p>Body</p>", MessageID: "one@example.test"})
	if err == nil || sent != "" {
		t.Fatal("an unsupported relay silently discarded staged markup")
	}
}
