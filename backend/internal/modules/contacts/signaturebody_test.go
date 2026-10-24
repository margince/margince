// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyTheSystemSweepMayReadASignatureBody(t *testing.T) {
	for _, actor := range []principal.PrincipalType{principal.PrincipalHuman, principal.PrincipalAgent} {
		t.Run(string(actor), func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), principal.Principal{Type: actor})
			body, readable, err := (&Store{}).ReadableSignatureBody(ctx, ids.ContactID{}, ids.UUID{}, true)
			if !errors.Is(err, apperrors.ErrPermissionDenied) || readable || body != "" {
				t.Fatalf("body=%q, readable=%v, err=%v; want permission denial without content", body, readable, err)
			}
		})
	}
}
