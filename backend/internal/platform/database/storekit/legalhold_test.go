// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A hold on a deal announced as contact.updated would reach the subscribers of
// a contact that does not exist. The writer refuses it before it reads.
func TestALegalHoldRefusesAnEventForAnotherRecordType(t *testing.T) {
	err := SetLegalHold(context.Background(), nil, "deal", ids.NewV7(), true, "Litigation",
		func(changed map[string]any) events.Payload {
			return crmcontracts.PublicEventContactUpdated{ChangedFields: changed}
		})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("got %v, want ErrInvalidArgument", err)
	}
}
