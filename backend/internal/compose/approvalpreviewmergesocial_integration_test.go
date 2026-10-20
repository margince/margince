// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactMergePreviewNamesDiscardedAndMovingSocialHandles(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	source, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Original contact", Source: "manual", Social: map[string]any{"linkedin": "original-profile", "github": "developer-profile"},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Surviving contact", Source: "manual", Social: map[string]any{"linkedin": "surviving-profile"},
	})
	if err != nil {
		t.Fatal(err)
	}
	from, to := ids.UUID(source.Id), ids.UUID(target.Id)
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Social) != 2 {
		t.Fatalf("social effects: %+v", preview.Social)
	}
	for _, effect := range preview.Social {
		if effect.SourceID.IsZero() {
			t.Fatal("source social row is unnamed")
		}
		switch effect.Platform {
		case "github":
			if effect.SourceHandle != "developer-profile" || effect.TargetID != nil || effect.TargetHandle != nil {
				t.Fatalf("moving handle: %+v", effect)
			}
		case "linkedin":
			if effect.SourceHandle != "original-profile" || effect.TargetID == nil || effect.TargetHandle == nil || *effect.TargetHandle != "surviving-profile" {
				t.Fatalf("discarded handle: %+v", effect)
			}
		default:
			t.Fatalf("unexpected platform: %+v", effect)
		}
	}
	checkMergePreviewRefusals(t, e, "contact", from, to, preview, "social")
	merged, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](from), ids.From[ids.ContactKind](to), &preview.SourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Social == nil || len(*merged.Social) != 2 || (*merged.Social)["linkedin"] != "surviving-profile" || (*merged.Social)["github"] != "developer-profile" {
		t.Fatalf("merged social handles: %+v", merged.Social)
	}
}
