// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestArchivingNamesTheFailedOwnershipRule(t *testing.T) {
	e := integration.Setup(t)
	member := e.As(e.Rep1, nil, principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}, "user_admin": {Read: true}}})
	create := func() ids.ContactID {
		row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Archive candidate", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.Rep2))})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.ContactKind](ids.UUID(row.Id))
	}
	other := create()
	_, err := e.Contacts.ArchiveContact(member, other, nil)
	fault, ok := httperr.Classify(err)
	if !ok || fault.Code != "archive_owner_required" {
		t.Fatalf("non-owner archive: %v (%+v)", err, fault)
	}
	self := create()
	if _, err := e.Contacts.UpdateContact(member, self, contacts.UpdateContactInput{OwnerID: new(ids.From[ids.UserKind](e.Rep1))}); err != nil {
		t.Fatal(err)
	}
	_, err = e.Contacts.ArchiveContact(member, self, nil)
	var refusal *auth.ArchiveAuthorityError
	if !errors.As(err, &refusal) || !refusal.SelfAssigned {
		t.Fatalf("self-assignment archive: %v", err)
	}
	if fault, ok := httperr.Classify(err); !ok || fault.Code != "archive_self_assigned" {
		t.Fatalf("self-assignment HTTP reason: %+v", fault)
	}
	if _, err := e.Contacts.ArchiveContact(e.Admin(), self, nil); err != nil {
		t.Fatalf("manager archive: %v", err)
	}
}
