// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestRelinkPreviewKeepsBlindRecipientsWithTheSender(t *testing.T) {
	e := Setup(t)
	sender := e.As(e.Rep1, nil, AdminPerms)
	reviewer := e.As(e.Rep2, nil, AdminPerms)
	blind := e.SeedContact(t, "Blind recipient", nil)
	target := e.SeedContact(t, "Correct recipient", nil)
	const address = "blind@example.test"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ContactIDOf(blind), contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}}}); err != nil {
		t.Fatal(err)
	}
	activity, _, err := e.Activities.LogActivity(sender, activities.LogActivityInput{Kind: "email", Subject: new("Private recipient list"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: blind}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(activity.Id))
	// Use the same participant writer as capture; no alternate preview fixture policy.
	if err := e.DB().Tx(sender, func(tx pgx.Tx) error {
		return capture.StampFurtherParticipants(sender, tx, id, "email", "", true, []connector.MessageParticipant{{Email: address, Role: connector.ParticipantRoleBCC}})
	}); err != nil {
		t.Fatal(err)
	}
	in := activities.RelinkActivityInput{EntityType: "contact", EntityID: target, ReplaceExistingOfType: true}
	plan, err := e.Activities.PreviewRelink(sender, []ids.UUID{id.UUID}, in)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, participant := range plan[0].Participants {
		if participant.Role == connector.ParticipantRoleBCC && participant.Address != nil && *participant.Address == address {
			found = true
		}
	}
	if !found {
		t.Fatalf("sender lost the blind recipient effect: %+v", plan)
	}
	denied, err := e.Activities.PreviewRelink(reviewer, []ids.UUID{id.UUID}, in)
	if !errors.Is(err, apperrors.ErrNotFound) || denied != nil {
		t.Fatalf("other reviewer received blind-copy scope: %+v, %v", denied, err)
	}
	in.ReplaceExistingOfType = false
	if _, err := e.Activities.PreviewRelink(reviewer, []ids.UUID{id.UUID}, in); err != nil {
		t.Fatalf("unaffected blind copy blocked association: %v", err)
	}
}
