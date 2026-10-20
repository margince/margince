// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMergePreviewMatchesFilesAndDrafts(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		for _, collision := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/collision=%t", kind, collision), func(t *testing.T) {
				e := integration.Setup(t)
				ctx := e.Admin()
				source, target := mergeContentPair(t, e, kind)
				store := e.Activities.WithBlobstore(blobstore.NewMemory())
				for _, archived := range []bool{false, true} {
					file, err := store.UploadAttachment(ctx, activities.AttachmentInput{EntityType: kind, EntityID: source, Filename: fmt.Sprintf("file-%t.txt", archived), ContentType: "text/plain", Content: bytes.NewReader([]byte("Synthetic attachment"))})
					if err != nil {
						t.Fatal(err)
					}
					if archived {
						if err := store.ArchiveAttachment(ctx, ids.UUID(file.Id)); err != nil {
							t.Fatal(err)
						}
					}
				}
				anchor := activities.MailDraftAnchor{Type: crmcontracts.MailDraftAnchorType(kind), ID: source}
				draft, err := store.SaveMailDraft(ctx, anchor, activities.MailDraftContent{To: []string{"buyer@example.test"}, Subject: "Source draft", Body: "Keep these words", HTMLBody: "<p>Keep these words</p>"}, nil)
				if err != nil {
					t.Fatal(err)
				}
				expected := draft
				if collision {
					expected, err = store.SaveMailDraft(ctx, activities.MailDraftAnchor{Type: anchor.Type, ID: target}, activities.MailDraftContent{Subject: "Survivor draft", Body: "Survivor words"}, nil)
					if err != nil {
						t.Fatal(err)
					}
				}
				preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(preview.Files) != 2 || preview.Draft == nil || ids.UUID(preview.Draft.Source.Id) != draft.ID || preview.Draft.Source.HtmlBody == nil || *preview.Draft.Source.HtmlBody != draft.Content.HTMLBody {
					t.Fatalf("missing scope: %+v", preview)
				}
				if (preview.Draft.Target != nil) != collision {
					t.Fatalf("wrong draft collision: %+v", preview.Draft)
				}
				if collision && (ids.UUID(preview.Draft.Target.Id) != expected.ID || preview.Draft.Target.Body != expected.Content.Body) {
					t.Fatalf("wrong survivor draft: %+v", preview.Draft)
				}
				e.Contacts.WithMergeContentReader(nil)
				partial, refusal := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
				if !errors.Is(refusal, apperrors.ErrInvalidArgument) || partial.Files != nil || partial.Draft != nil {
					t.Fatalf("unwired scope: %+v %v", partial, refusal)
				}
				e.Contacts.WithMergeContentReader(store)
				if kind == "contact" {
					_, err = e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion)
				} else {
					_, err = e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), &preview.SourceVersion)
				}
				if err != nil {
					t.Fatal(err)
				}
				actual, err := store.GetMailDraft(ctx, activities.MailDraftAnchor{Type: anchor.Type, ID: target})
				if err != nil || actual.ID != expected.ID || actual.Content.Body != expected.Content.Body {
					t.Fatalf("actual draft: %+v, %v", actual, err)
				}
				if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
					for _, file := range preview.Files {
						var parent ids.UUID
						var archived bool
						var checksum string
						if err := tx.QueryRow(ctx, `SELECT entity_id, archived_at IS NOT NULL, checksum FROM attachment WHERE id=@id`, pgx.StrictNamedArgs{"id": file.ID}).Scan(&parent, &archived, &checksum); err != nil {
							return err
						}
						if parent != target || archived != (file.ArchivedAt != nil) || file.Checksum == nil || checksum != *file.Checksum {
							t.Errorf("file did not move intact: %+v", file)
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestMergePreviewWithholdsAnotherAuthorsDraft(t *testing.T) {
	e := integration.Setup(t)
	source, target := mergeContentPair(t, e, "contact")
	owner := e.As(e.Rep1, nil, integration.AdminPerms)
	draft, err := e.Activities.SaveMailDraft(owner, activities.MailDraftAnchor{Type: "contact", ID: source}, activities.MailDraftContent{Subject: "Private draft", Body: "Private words"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := e.Contacts.PreviewMergeSurvivorship(e.Admin(), "contact", source, target, nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) || hidden.Draft != nil || hidden.Files != nil || hidden.Fields != nil {
		t.Fatalf("private draft leaked: %+v %v", hidden, err)
	}
	visible, err := e.Contacts.PreviewMergeSurvivorship(owner, "contact", source, target, nil, nil)
	if err != nil || visible.Draft == nil || ids.UUID(visible.Draft.Source.Id) != draft.ID {
		t.Fatalf("author cannot review draft: %+v %v", visible, err)
	}
}

func TestMergePreviewRefusesLostDraftGroundingWithoutDiscarding(t *testing.T) {
	e := integration.Setup(t)
	source, target := mergeContentPair(t, e, "contact")
	colleague := e.As(e.Rep1, nil, integration.AdminPerms)
	activity, _, err := e.Activities.LogActivity(colleague, activities.LogActivityInput{Kind: "note", Subject: new("Evidence"), Body: new("Shared initially")})
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(e.Admin())
	if !ok {
		t.Fatal("missing principal")
	}
	actor.Type, actor.ID = principal.PrincipalAgent, "agent:synthetic-draft"
	anchor := activities.MailDraftAnchor{Type: "contact", ID: source}
	draft, err := e.Activities.SaveAgentMailDraft(principal.WithActor(e.Admin(), actor), anchor, activities.MailDraftContent{Subject: "Grounded draft", Body: "Derived words"}, []activities.MailDraftAnchor{{Type: "activity", ID: ids.UUID(activity.Id)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.SetAudience(colleague, ids.From[ids.ActivityKind](ids.UUID(activity.Id)), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}}}); err != nil {
		t.Fatal(err)
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(e.Admin(), "contact", source, target, nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) || preview.Draft != nil || preview.Fields != nil {
		t.Fatalf("lost grounding served: %+v %v", preview, err)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(e.Admin(), `SELECT count(*) FROM mail_draft WHERE id=@id`, pgx.StrictNamedArgs{"id": draft.ID}).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Error("preview mutated the saved draft")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func mergeContentPair(t *testing.T, e *integration.Env, kind string) (ids.UUID, ids.UUID) {
	t.Helper()
	var records []ids.UUID
	for _, name := range []string{"Original record", "Surviving record"} {
		if kind == "contact" {
			record, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			records = append(records, ids.UUID(record.Id))
		} else {
			record, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			records = append(records, ids.UUID(record.Id))
		}
	}
	return records[0], records[1]
}
