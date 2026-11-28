// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyAHumanWithTheConfigGrantChangesAPurpose(t *testing.T) {
	for _, kind := range []principal.PrincipalType{principal.PrincipalHuman, principal.PrincipalAgent, principal.PrincipalConnector, principal.PrincipalSystem} {
		t.Run(string(kind), func(t *testing.T) {
			permissions := principal.Permissions{Objects: map[string]principal.ObjectGrant{"consent_config": {Update: kind != principal.PrincipalHuman}}}
			ctx := principal.WithActor(context.Background(), principal.Principal{Type: kind, ID: "test", Permissions: permissions})
			store := NewStore(nil)
			id := ids.New[ids.PurposeKind]()
			if _, err := store.RenamePurpose(ctx, id, "Another label"); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("rename admitted %s: %v", kind, err)
			}
			if err := store.ArchivePurpose(ctx, id); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("archive admitted %s: %v", kind, err)
			}
		})
	}
}
