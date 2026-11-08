// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A contact names the note it was read out of, and only to a reader who may
// open that note. The create refuses a source the caller cannot read, as not
// found, and a read serves the source to nobody the note's audience leaves out.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sourceReader binds one seat, as a human or as the agent acting for it, with
// or without the activity read grant. The grant is the field's object half.
func (e *privacyEnv) sourceReader(user ids.UUID, kind principal.PrincipalType, readsActivity bool) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	objects := map[string]principal.ObjectGrant{
		"contact": {Create: true, Read: true, Update: true},
		"company": {Create: true, Read: true},
	}
	if readsActivity {
		objects["activity"] = principal.ObjectGrant{Read: true}
	}
	actor := principal.Principal{
		Type: kind, ID: "human:" + user.String(), UserID: user,
		TeamIDs: []ids.UUID{e.team},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, Objects: objects, RowScope: principal.RowScopeAll,
		},
	}
	if kind == principal.PrincipalAgent {
		actor.ID, actor.OnBehalfOf = "agent:tools", user
	}
	return principal.WithActor(ctx, actor)
}

// ownersNote writes a note the owner logged for themselves alone: link-less, so
// every seat discovers it, and audience `participants`, so only its author
// reads it.
func (e *privacyEnv) ownersNote(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	ctx := e.as(e.owner, principal.RowScopeAll)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
			VALUES ($1, 'note', 'Met Lucy at the fair', now(), 'manual', $2, 'participants')`,
			id, "human:"+e.owner.String())
		return err
	}); err != nil {
		t.Fatalf("seeding the owner's note: %v", err)
	}
	return id
}

func (e *privacyEnv) createFrom(ctx context.Context, source ids.UUID, email string) (crmcontracts.Contact, error) {
	return e.store.CreateContact(ctx, CreateContactInput{
		FullName:              "Lucy Vo",
		Emails:                []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		Source:                "manual",
		CreatedFromActivityID: &source,
	})
}

func TestAContactAnAgentReadsOutOfANoteNamesThatNote(t *testing.T) {
	e := setupCapturePrivacy(t)
	note := e.ownersNote(t)

	created, err := e.createFrom(e.sourceReader(e.owner, principal.PrincipalAgent, true), note, "lucy.note@kunde.example")
	if err != nil {
		t.Fatalf("creating a contact from the owner's own note: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))

	got, err := e.store.GetContact(e.sourceReader(e.owner, principal.PrincipalHuman, true), id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("the owner reading the contact back: %v", err)
	}
	if got.CreatedFromActivityId == nil || ids.UUID(*got.CreatedFromActivityId) != note {
		t.Errorf("created_from_activity_id = %v, want the note %s: the reader cannot check where the name came from",
			got.CreatedFromActivityId, note)
	}
}

func TestACreateNamingANoteTheCallerCannotReadIsNotFound(t *testing.T) {
	e := setupCapturePrivacy(t)
	note := e.ownersNote(t)

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"a seat outside the note's audience", e.sourceReader(e.teammate, principal.PrincipalAgent, true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.createFrom(tc.ctx, note, "lucy."+ids.NewV7().String()+"@kunde.example")
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("create = %v, want not found: a refusal that is not a 404 says the note exists", err)
			}
		})
	}
}

func TestTheSourceIsWithheldFromAReaderWhoCannotOpenIt(t *testing.T) {
	e := setupCapturePrivacy(t)
	note := e.ownersNote(t)
	created, err := e.createFrom(e.sourceReader(e.owner, principal.PrincipalAgent, true), note, "lucy.held@kunde.example")
	if err != nil {
		t.Fatalf("creating a contact from the owner's own note: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"a colleague outside the note's audience", e.sourceReader(e.teammate, principal.PrincipalHuman, true)},
		{"the owner without the activity read grant", e.sourceReader(e.owner, principal.PrincipalHuman, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.store.GetContact(tc.ctx, id, storekit.LiveOnly)
			if err != nil {
				t.Fatalf("the contact itself must still read: %v", err)
			}
			if got.CreatedFromActivityId != nil {
				t.Errorf("created_from_activity_id = %s, want none: the id says a record exists this reader cannot open",
					*got.CreatedFromActivityId)
			}
		})
	}
}

// The approval preview answers what the write will answer. A staged create
// naming a note its human cannot read must not preview as ready.
func TestThePreviewRefusesASourceItsCreatorCannotRead(t *testing.T) {
	e := setupCapturePrivacy(t)
	note := e.ownersNote(t)
	creator, ok := principal.Actor(e.sourceReader(e.teammate, principal.PrincipalHuman, true))
	if !ok {
		t.Fatal("the creator context carries no actor")
	}
	in := CreateContactInput{
		FullName:              "Lucy Vo",
		Emails:                []ContactEmailInput{{Email: "lucy.preview@kunde.example", EmailType: "work", IsPrimary: true}},
		Source:                "manual",
		CreatedFromActivityID: &note,
	}

	_, err := e.store.PreviewContactCreationForActor(e.sourceReader(e.teammate, principal.PrincipalAgent, true), in, creator)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("preview = %v, want not found: the approved write refuses this source", err)
	}
}
