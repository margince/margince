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

func taggerWith(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:tagger",
		Permissions: principal.Permissions{RoleKeys: []string{"fixture"}, Objects: objects},
	})
}

// Applying a tag writes to the RECORD, so it needs UPDATE on the record — not
// merely read. A seat that may look at a project and not change it must not be
// able to hang a tag on it, because a tag steers who finds that project in a
// filter and what an automation does with it.
//
// The case is chosen to fail under the OLD pairing too: read on the target is
// present and update is missing, which the previous rule admitted.
func TestApplyingATagNeedsUpdateOnTheTargetNotMerelyRead(t *testing.T) {
	store := NewStore(nil)
	ctx := taggerWith(map[string]principal.ObjectGrant{
		"tag":     {Read: true},
		"project": {Read: true},
	})
	_, err := store.ApplyTag(ctx, ids.New[ids.TagKind](), "project", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat holding project.read but not project.update applied a tag: %v", err)
	}
}

// The other half of the inversion: vocabulary authority is no longer what
// admits an application. A seat that may apply tags does NOT thereby hold the
// right to coin them, and this proves the gate asks for tag.read rather than
// tag.update — a rep holds the first and not the second.
func TestApplyingATagNeedsOnlyReadOnTheVocabulary(t *testing.T) {
	store := NewStore(nil)
	ctx := taggerWith(map[string]principal.ObjectGrant{
		"tag":     {Read: true},
		"project": {Read: true, Update: true},
	})
	// nil pool: the permission gates run before any query, so reaching the
	// database is itself the assertion that both gates admitted this caller.
	// A refusal here would be a permission refusal, which is what fails the
	// test; anything else is the nil pool and means the gates passed.
	_, err := store.ApplyTag(ctx, ids.New[ids.TagKind](), "project", ids.NewV7())
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat with tag.read and project.update was refused; applying must not require tag.update: %v", err)
	}
}

// Removing is the same write and carries the same pair.
func TestRemovingATagNeedsUpdateOnTheTarget(t *testing.T) {
	store := NewStore(nil)
	ctx := taggerWith(map[string]principal.ObjectGrant{
		"tag":     {Read: true},
		"project": {Read: true},
	})
	err := store.RemoveTag(ctx, ids.New[ids.TagKind](), "project", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat holding project.read but not project.update removed a tag: %v", err)
	}
}
