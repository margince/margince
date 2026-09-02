// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// ResolveTag never creates and never resurrects. These tests drive the REAL
// seam against a real database, because that is where the rule lives: a test
// against a stub proves only what the stub was written to do.

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

	_, err = tagSeam(e.Pool).ResolveTag(ctx, "Repro Tag")
	if err == nil {
		t.Fatal("ResolveTag on an archived-only name succeeded; want a refusal — an archived word was retired on purpose")
	}
	// The refusal has to say the word EXISTS and is retired. "No such tag"
	// would send a rep to ask for a tag the workspace already has, and an
	// admin would then coin a duplicate of a word they deliberately retired.
	if !strings.Contains(err.Error(), "archived") {
		t.Fatalf("ResolveTag's refusal does not say the name is archived, so a caller cannot tell it from an unknown word: %v", err)
	}
}

// The governance rule at the seam that enforces it: an unknown name is a
// refusal, and nothing is written. This is the half a stub cannot prove — the
// tag table has to still be empty afterwards.
func TestResolveTagRefusesAnUnknownNameAndCoinsNothing(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, tagAdminPerms)
	store := collections.NewStore(InstallationDB(e.Pool))

	_, err := tagSeam(e.Pool).ResolveTag(ctx, "Never Coined")
	if err == nil {
		t.Fatal("an unknown name resolved; want a refusal, because coining one here is the authority only admin and ops hold")
	}

	// The word must not exist now. A seam that created it and then returned
	// the id would also have answered without error above, so the absence is
	// the assertion that separates the two.
	if _, found, findErr := store.FindTag(ctx, "Never Coined"); findErr != nil {
		t.Fatalf("looking the name up afterwards: %v", findErr)
	} else if found {
		t.Error("the refused name exists as a tag; ResolveTag coined the word it was asked to refuse")
	}
}
