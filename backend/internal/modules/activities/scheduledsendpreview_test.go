// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestScheduledPreviewPreservesFrozenMessageAndRefusesUnknownFormats(t *testing.T) {
	input := SendEmailInput{
		Subject: "Requested specification", Body: "Full specification attached.",
		HTMLBody:   "<p>Full specification attached.</p>",
		Recipients: []string{"buyer@example.test"}, Cc: []string{"cc@example.test"},
		Bcc: []string{"archive@example.test"}, AttachmentIDs: []ids.UUID{ids.NewV7()},
		Context: commsauthz.CategoryMarketing, MarketingPurpose: "newsletter",
		OperatorReason: "Requested updates", Evidence: commsauthz.Evidence{ConsentEventID: ids.NewV7()},
	}
	message := ScheduledSend{PayloadVersion: payloadVersionCurrent, frozen: freezePayload(input)}
	actual, links, err := message.PreviewMessage()
	if err != nil || !reflect.DeepEqual(actual, input) || len(links) != 0 {
		t.Fatalf("preview narrowed message: %+v / %+v / %v", actual, links, err)
	}
	for _, version := range []int{0, 1, payloadVersionCurrent + 1} {
		message.PayloadVersion = version
		actual, links, err = message.PreviewMessage()
		if !errors.Is(err, apperrors.ErrInvalidArgument) || actual.Body != "" || len(links) != 0 {
			t.Fatalf("payload version %d exposed context: %+v / %v", version, actual, err)
		}
	}
	message.PayloadVersion = payloadVersionCurrent
	message.additionalLinks = []byte(`{"not":"a list"}`)
	if _, _, err := message.PreviewMessage(); err == nil {
		t.Fatal("malformed filing accepted as complete context")
	}
}
