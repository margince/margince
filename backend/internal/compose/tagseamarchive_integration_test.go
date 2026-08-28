// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// EnsureTag's own comment says an archived name is never silently reused —
// that decision is deliberate and this test does not touch it. What it
// guards is the error that decision produces: it must not direct the caller
// to an action the contract cannot perform.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The harness's own AdminPerms deliberately narrows `tag` to Read only (it
// is not a mirror of the seeded admin role), so creating and archiving the
// fixture here needs its own grant.
var tagAdminPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects:  map[string]principal.ObjectGrant{"tag": {Create: true, Read: true, Update: true, Delete: true}},
	RowScope: principal.RowScopeAll,
}

func TestEnsureTagOnAnArchivedNameDoesNotPromiseARestoreThatDoesNotExist(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, tagAdminPerms)
	store := collections.NewStore(InstallationDB(e.Pool))

	created, err := store.NewTag(ctx, "Repro Tag", "")
	if err != nil {
		t.Fatalf("creating the fixture tag: %v", err)
	}
	if _, err := store.ArchiveTag(ctx, ids.From[ids.TagKind](created.TagID)); err != nil {
		t.Fatalf("archiving the fixture tag: %v", err)
	}

	_, err = tagSeam(e.Pool).EnsureTag(ctx, "Repro Tag")
	if err == nil {
		t.Fatal("EnsureTag on an archived-only name succeeded; want a conflict — the vocabulary deliberately does not reuse an archived word")
	}
	if strings.Contains(err.Error(), "restore") {
		t.Fatalf("EnsureTag's error directs the caller to %q, which the contract has no operation for: %v",
			"restore", err)
	}
	if !strings.Contains(err.Error(), "is not reused") {
		t.Fatalf("EnsureTag's error does not explain why the name was refused: %v", err)
	}
}
