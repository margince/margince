// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRelinkPreviewMatchesReplacementAndAssociation(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "associate", true: "replace"}[replace], func(t *testing.T) {
			e := Setup(t)
			perms := AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			var targets []ids.UUID
			for _, name := range []string{"Initial contact", "Correct contact"} {
				contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				targets = append(targets, ids.UUID(contact.Id))
			}
			activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
				Kind: "note", Subject: new("Buyer discussion"), Source: "manual",
				Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: targets[0]}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if activity.Version == nil {
				t.Fatal("missing activity version")
			}
			id := ids.From[ids.ActivityKind](ids.UUID(activity.Id))
			in := activities.RelinkActivityInput{EntityType: "contact", EntityID: targets[1], ReplaceExistingOfType: replace, IfVersion: new(int64(*activity.Version))}
			preview, err := e.Activities.PreviewRelink(ctx, []ids.UUID{id.UUID}, in)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview) != 1 || !preview[0].AddsLink || preview[0].StampsCorrespondence {
				t.Fatalf("wrong effect: %+v", preview)
			}
			if replace && (len(preview[0].Removed) != 1 || preview[0].Removed[0] != targets[0]) {
				t.Fatalf("wrong removed set: %+v", preview)
			}
			if !replace && len(preview[0].Removed) != 0 {
				t.Fatalf("association removes links: %+v", preview)
			}
			after, err := e.Activities.RelinkActivity(ctx, id, in)
			if err != nil {
				t.Fatalf("preview changed the pinned activity: %v", err)
			}
			want := 2
			if replace {
				want = 1
			}
			if after.Links == nil || len(*after.Links) != want {
				t.Fatalf("writer disagrees with preview: %+v", after.Links)
			}
			if _, err := e.Activities.PreviewRelink(ctx, []ids.UUID{id.UUID}, in); !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("stale preview: %v", err)
			}
			in.IfVersion = nil
			in.ReplaceExistingOfType = false
			noop, err := e.Activities.PreviewRelink(ctx, []ids.UUID{id.UUID, id.UUID}, in)
			if err != nil {
				t.Fatal(err)
			}
			if len(noop) != 1 || noop[0].AddsLink {
				t.Fatalf("duplicate/no-op scope: %+v", noop)
			}
		})
	}
}
