// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// An update merges `social` into the stored links. A caller fixing one link
// must not erase the others, and the audit row names only what moved.

import (
	"context"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const mergedProfile = "https://www.linkedin.com/in/ada-merge"

func (e *dedupeEnv) contactWithLinks(ctx context.Context, t *testing.T) ids.ContactID {
	t.Helper()
	created, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Merge", Source: "manual",
		Social: map[string]any{SocialLinkedIn: mergedProfile, "github": "ada-merge"},
	})
	if err != nil {
		t.Fatalf("create the contact: %v", err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

func (e *dedupeEnv) socialOf(ctx context.Context, t *testing.T, id ids.ContactID) map[string]any {
	t.Helper()
	got, err := e.store.GetContact(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the contact back: %v", err)
	}
	if got.Social == nil {
		return map[string]any{}
	}
	return *got.Social
}

func TestAnUpdateNamingOneSocialLinkKeepsTheOthers(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	id := e.contactWithLinks(ctx, t)

	if _, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Social: map[string]any{"twitter": "ada"}}); err != nil {
		t.Fatalf("add a twitter handle: %v", err)
	}

	want := map[string]any{SocialLinkedIn: mergedProfile, "github": "ada-merge", "twitter": "ada"}
	if got := e.socialOf(ctx, t, id); !reflect.DeepEqual(got, want) {
		t.Errorf("social = %v, want %v: an update that named only twitter changed the other links", got, want)
	}
	before, after := auditImagesHolding(ctx, t, e.store, entityContact, id.UUID, "social")
	if !reflect.DeepEqual(after["social"], map[string]any{"twitter": "ada"}) ||
		!reflect.DeepEqual(before["social"], map[string]any{"twitter": nil}) {
		t.Errorf("audit images = %v → %v, want only the twitter key that moved", before["social"], after["social"])
	}
}

func TestAnEmptyOrNullSocialValueRemovesThatLink(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	id := e.contactWithLinks(ctx, t)

	if _, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Social: map[string]any{SocialLinkedIn: ""}}); err != nil {
		t.Fatalf("clear the LinkedIn link: %v", err)
	}
	if got := e.socialOf(ctx, t, id); !reflect.DeepEqual(got, map[string]any{"github": "ada-merge"}) {
		t.Errorf("social after an empty linkedin = %v, want only github left", got)
	}
	if _, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Social: map[string]any{"github": nil}}); err != nil {
		t.Fatalf("clear the github handle: %v", err)
	}
	if got := e.socialOf(ctx, t, id); len(got) != 0 {
		t.Errorf("social after a null github = %v, want no links", got)
	}
}
