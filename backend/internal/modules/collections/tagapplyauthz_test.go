// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Tagging a record is a read of it, on BOTH doors. The tag_name path asked the
// target's object type through EnsureTaggable and the direct tag_id path did
// not, so a role holding tag.update without <type>.read could tag rows it may
// not see. The refusal runs before any query, so this probe needs no database;
// the admitting half — the same call succeeding under a principal that holds
// the read — is every ApplyTag use in the integration lane, which turns red if
// this gate ever over-refuses.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestADirectTagIDApplyStillAsksTheTargetsObjectType(t *testing.T) {
	store := NewStore(nil)
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:tagger",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects: map[string]principal.ObjectGrant{
				"tag": {Read: true, Update: true},
			},
		},
	})
	_, err := store.ApplyTag(ctx, ids.New[ids.TagKind](), "project", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("tag.update without project.read applied a tag by id: %v", err)
	}
}
