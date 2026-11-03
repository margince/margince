// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLeadCreationPreviewMatchesActorDefaultsAndNormalization(t *testing.T) {
	for _, agent := range []bool{false, true} {
		name := "human"
		if agent {
			name = "agent"
		}
		t.Run(name, func(t *testing.T) {
			e := setupPromoteConsent(t)
			ctx := e.ctx
			if agent {
				actor, ok := principal.Actor(ctx)
				if !ok {
					t.Fatal("missing fixture actor")
				}
				actor.Type, actor.ID, actor.UserID = principal.PrincipalAgent, "agent:qualification", e.user
				ctx = principal.WithActor(ctx, actor)
			}
			in := CreateLeadInput{FullName: new("New Buyer"), Email: new("BUYER@EXAMPLE.TEST"), Source: "manual"}
			preview, err := e.store.PreviewLeadCreation(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Existing != nil || preview.Input.Email == nil || *preview.Input.Email != "buyer@example.test" || preview.Input.Status != "new" {
				t.Fatalf("defaults: %+v", preview)
			}
			created, inserted, err := e.store.CreateLead(ctx, in)
			if err != nil || !inserted {
				t.Fatalf("preview wrote a row or writer refused: %v, %v", inserted, err)
			}
			if int(created.Score) != preview.Score {
				t.Fatalf("score differs: %v, %v", created.Score, preview.Score)
			}
			if created.Email == nil || string(*created.Email) != *preview.Input.Email || string(created.Status) != preview.Input.Status {
				t.Fatalf("writer differs: %+v, %+v", created, preview)
			}
			if agent {
				if preview.Input.OwnerID == nil || preview.Input.OwnerID.UUID != e.user || created.OwnerId == nil || ids.UUID(*created.OwnerId) != e.user {
					t.Fatal("agent owner default differs")
				}
			} else if preview.Input.OwnerID != nil || created.OwnerId != nil {
				t.Fatal("human lead should remain unassigned")
			}
			_, err = e.store.PreviewLeadCreation(ctx, in)
			if _, ok := errors.AsType[*DuplicateLeadError](err); !ok {
				t.Fatalf("duplicate admission: %v", err)
			}
		})
	}
}

func TestLeadCreationPreviewRefusesTheDeprecatedContactID(t *testing.T) {
	e := setupPromoteConsent(t)
	contact, err := e.store.CreateContact(e.ctx, CreateContactInput{FullName: "Known Buyer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	in := CreateLeadInput{RequestedContactID: new(ids.From[ids.ContactKind](ids.UUID(contact.Id))), Source: "manual"}
	for name, err := range map[string]error{
		"preview": func() error { _, err := e.store.PreviewLeadCreation(e.ctx, in); return err }(),
		"create":  func() error { _, _, err := e.store.CreateLead(e.ctx, in); return err }(),
	} {
		var invalid *httperr.DetailedError
		if !errors.As(err, &invalid) || len(invalid.Fields) != 1 || invalid.Fields[0].Field != contactIDField {
			t.Errorf("%s contact_id = %v, want a field refusal independent of the supplied row", name, err)
		}
	}
}

func TestLeadCreationPreviewNamesTheDuplicateReviewEffect(t *testing.T) {
	e := setupPromoteConsent(t)
	existing, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{FullName: new("Dana Buyer"), CompanyName: new("Example buyer"), Email: new("first@example.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	in := CreateLeadInput{FullName: new("Dana Buyer"), CompanyName: new("Example buyer"), Email: new("second@example.test"), Source: "manual"}
	preview, err := e.store.PreviewLeadCreation(e.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ReviewLead == nil || preview.ReviewLead.Id != existing.Id {
		t.Fatalf("missing duplicate review: %+v", preview)
	}
	created, inserted, err := e.store.CreateLead(e.ctx, in)
	if err != nil || !inserted {
		t.Fatalf("create: %v,%v", inserted, err)
	}
	var count int
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM dedupe_candidate WHERE entity_type='lead' AND
   ((left_lead_id=@created AND right_lead_id=@existing) OR (left_lead_id=@existing AND right_lead_id=@created))`, pgx.NamedArgs{"created": created.Id, "existing": existing.Id}).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("writer recorded %d duplicate pairs", count)
	}
}

func TestLeadCreationPreviewUsesCreatorDefaultsWithoutBorrowingPermissions(t *testing.T) {
	e := setupPromoteConsent(t)
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	creator := principal.Principal{Type: principal.PrincipalAgent, ID: "agent:qualification", UserID: e.user}
	in := CreateLeadInput{FullName: new("New buyer"), Source: "manual"}
	preview, err := e.store.PreviewLeadCreationForActor(e.ctx, in, creator)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Input.OwnerID == nil || preview.Input.OwnerID.UUID != e.user {
		t.Fatalf("lost creator's owner: %+v", preview)
	}
	human := principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user}
	preview, err = e.store.PreviewLeadCreationForActor(e.ctx, in, human)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Input.OwnerID != nil {
		t.Fatalf("human create inherited reviewer defaults: %+v", preview)
	}
	actor.Permissions.Objects = map[string]principal.ObjectGrant{leadEntity: {Read: true}}
	denied := principal.WithActor(e.ctx, actor)
	creator.Permissions.Objects = map[string]principal.ObjectGrant{leadEntity: {Create: true, Read: true, Update: true}}
	creator.Permissions.RowScope = principal.RowScopeAll
	if _, err := e.store.PreviewLeadCreationForActor(denied, in, creator); err == nil {
		t.Fatal("creator permissions widened reviewer access")
	}
}
