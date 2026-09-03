// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The staging decision's shape, and the refusals it makes before touching a
// database.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A message with no recipients has not been authorized; it has failed to ask.
// Reported as the caller defect it is rather than as a suppression, because
// dressing it up would park the send with a reason an operator reads as "this
// person opted out" for a bug that named nobody.
func TestStagingRefusesAMessageWithNoRecipients(t *testing.T) {
	_, err := (&Gate{}).AuthorizeStagingTx(context.Background(), nil, ids.NewV7(), commsauthz.Request{})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("err = %v, want an invalid-argument fault for a message addressed to nobody", err)
	}
}

// A recipient carrying both an address and a channel identity would be answered
// about on the channel arm alone, and the address — which may be the one that
// objected — would never be looked at. The legacy gate refuses the same shape
// for the same reason.
func TestStagingRefusesAMalformedRecipient(t *testing.T) {
	both := connector.Recipient{
		Email:   "someone@example.test",
		Channel: &connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "42"},
	}
	_, err := (&Gate{}).AuthorizeStagingTx(context.Background(), nil, ids.NewV7(),
		commsauthz.Request{Recipients: []connector.Recipient{both}})
	if err == nil {
		t.Fatal("a recipient naming two subjects must be refused")
	}
	if errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Error("a malformed recipient is a caller defect, not an answer about anybody's consent")
	}
}
