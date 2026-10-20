// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMergePreviewMatchesMovedAndCombinedLinks(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			for _, object := range []string{"list", "tag"} {
				perms.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			}
			ctx := e.As(e.AdminUser, nil, perms)
			seed := e.SeedContact
			if kind == "company" {
				seed = e.SeedCompany
			}
			source, target := seed(t, "Source record", &e.AdminUser), seed(t, "Survivor record", &e.AdminUser)
			collection := collections.NewStore(e.DB())
			expected := map[ids.UUID]contacts.MergeLink{}
			for _, duplicate := range []bool{false, true} {
				links := []activities.ActivityLinkInput{{EntityType: kind, EntityID: source}}
				if duplicate {
					links = append(links, activities.ActivityLinkInput{EntityType: kind, EntityID: target})
				}
				activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new(fmt.Sprintf("Merge history %t", duplicate)), Links: links})
				if err != nil {
					t.Fatal(err)
				}
				expected[ids.UUID(activity.Id)] = contacts.MergeLink{Kind: "activity", ID: ids.UUID(activity.Id), Collapsed: duplicate}
				list, err := collection.CreateList(ctx, collections.CreateListInput{Name: fmt.Sprintf("Merge list %t", duplicate), EntityType: kind})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := collection.AddMember(ctx, list.ID, collections.MemberChange{EntityType: kind, EntityID: source, Reason: collections.ReasonChosen, Note: new("Keep source note")}); err != nil {
					t.Fatal(err)
				}
				if duplicate {
					if _, err := collection.AddMember(ctx, list.ID, collections.MemberChange{EntityType: kind, EntityID: target, Reason: collections.ReasonChosen}); err != nil {
						t.Fatal(err)
					}
				}
				expected[list.ID.UUID] = contacts.MergeLink{Kind: "list", ID: list.ID.UUID, Collapsed: duplicate, Note: new("Keep source note")}
				tag, err := collection.CreateTag(ctx, fmt.Sprintf("Merge tag %t", duplicate), nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := collection.ApplyTag(ctx, tag.ID, kind, source); err != nil {
					t.Fatal(err)
				}
				if duplicate {
					if _, err := collection.ApplyTag(ctx, tag.ID, kind, target); err != nil {
						t.Fatal(err)
					}
				}
				expected[tag.ID.UUID] = contacts.MergeLink{Kind: "tag", ID: tag.ID.UUID, Collapsed: duplicate}
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Links) != len(expected) {
				t.Fatalf("links=%+v", preview.Links)
			}
			for _, link := range preview.Links {
				want, ok := expected[link.ID]
				if !ok || link.Kind != want.Kind || link.Collapsed != want.Collapsed || (link.Note == nil) != (want.Note == nil) {
					t.Fatalf("link=%+v want=%+v", link, want)
				}
				if link.Note != nil && *link.Note != *want.Note {
					t.Errorf("note=%s", *link.Note)
				}
			}
			denied := perms
			denied.Objects = maps.Clone(perms.Objects)
			denied.Objects["activity"] = principal.ObjectGrant{}
			hidden, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, denied), kind, source, target, nil, nil)
			if err == nil || hidden.Fields != nil || hidden.Links != nil {
				t.Fatalf("hidden activity scope=%+v err=%v", hidden, err)
			}
			if kind == "contact" {
				if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion); err != nil {
					t.Fatal(err)
				}
			} else if _, err := e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			checkMergedLinks(t, e, ctx, kind, source, target, expected)
		})
	}
}

func checkMergedLinks(t *testing.T, e *integration.Env, ctx context.Context, kind string, source, target ids.UUID, expected map[ids.UUID]contacts.MergeLink) {
	t.Helper()
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		for id, link := range expected {
			var count, onTarget int
			args := pgx.StrictNamedArgs{"id": id, "source": source, "target": target, "kind": kind}
			var query string
			switch link.Kind {
			case "activity":
				query = `SELECT count(*), count(*) FILTER (WHERE contact_id=@target OR company_id=@target) FROM activity_link WHERE activity_id=@id AND entity_type=@kind AND (contact_id IN (@source,@target) OR company_id IN (@source,@target))`
			case "list":
				query = `SELECT count(*), count(*) FILTER (WHERE entity_id=@target) FROM list_member WHERE list_id=@id AND entity_type=@kind AND entity_id IN (@source,@target)`
			case "tag":
				query = `SELECT count(*), count(*) FILTER (WHERE entity_id=@target) FROM taggable WHERE tag_id=@id AND entity_type=@kind AND entity_id IN (@source,@target)`
			default:
				return fmt.Errorf("unexpected kind %s", link.Kind)
			}
			if err := tx.QueryRow(ctx, query, args).Scan(&count, &onTarget); err != nil {
				return err
			}
			if count != 1 || onTarget != 1 {
				t.Errorf("%s combined count=%d on survivor=%d", link.Kind, count, onTarget)
			}
			if link.Kind == "list" {
				var note *string
				if err := tx.QueryRow(ctx, `SELECT note FROM list_member WHERE list_id=@id AND entity_type=@kind AND entity_id=@target`, pgx.StrictNamedArgs{"id": id, "kind": kind, "target": target}).Scan(&note); err != nil {
					return err
				}
				if note == nil || *note != *link.Note {
					t.Errorf("carried note=%v", note)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMergePreviewRefusesAHiddenListWithoutPartialScope(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["list"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	source, target := e.SeedContact(t, "Merge source", &e.AdminUser), e.SeedContact(t, "Merge survivor", &e.AdminUser)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, perms)
	collection := collections.NewStore(e.DB())
	list, err := collection.CreateList(colleague, collections.CreateListInput{Name: "Private colleague list", EntityType: "contact", Sharing: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collection.AddMember(colleague, list.ID, collections.MemberChange{EntityType: "contact", EntityID: source, Note: new("Private membership note"), Reason: collections.ReasonChosen}); err != nil {
		t.Fatal(err)
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source, target, nil, nil)
	if err == nil || preview.Fields != nil || preview.Links != nil || preview.SourceVersion != 0 {
		t.Fatalf("hidden list disclosed: %+v err=%v", preview, err)
	}
}
