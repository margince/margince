// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPrivateContactCannotClearItsOwnerIndependently(t *testing.T) {
	e := setupCapturePrivacy(t)
	id := e.captureContact(t, "owner")
	ctx := e.as(e.owner, principal.RowScopeOwn)
	_, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Clear: []string{"owner_id"}})
	var required *RequiredFieldError
	if !errors.As(err, &required) || required.Field != "owner_id" {
		t.Fatalf("clearing a private contact's owner: got %v, want required owner", err)
	}
	if owner := e.ownerOf(t, id); owner == nil || *owner != e.owner {
		t.Fatalf("refused owner clear changed owner: %v", owner)
	}
}
