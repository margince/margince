// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The transaction is nil, so any statement would panic: an error return is
// the proof that nothing was read or written for a call no seat made.
func TestAShareCallWithNoSeatBehindItIsRefusedBeforeAnyStatement(t *testing.T) {
	store := NewAnalyticsShareStore(time.Now)

	if _, err := store.ListIssued(context.Background(), nil); err == nil {
		t.Error("listing shares with no actor answered, want a refusal — the list is somebody's own")
	}
	if err := store.Revoke(context.Background(), nil, ids.NewV7()); err == nil {
		t.Error("closing a share with no actor answered, want a refusal — only the issuer closes a link")
	}
}

func TestAForecastReaderWhoCannotIssueCannotCloseAShare(t *testing.T) {
	reader := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"forecast": {Read: true}},
		},
	})

	err := NewAnalyticsShareStore(time.Now).Revoke(reader, nil, ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without forecast create closing a share answered %v, want a refusal", err)
	}
}
