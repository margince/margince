// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactAttachableFlagMatchesTheAttachGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		private bool
		share   string
		owner   bool
		want    bool
	}{
		{name: "read share", private: true, share: "read"},
		{name: "write share", private: true, share: "write", want: true},
		{name: "owner", private: true, owner: true, want: true},
		{name: "workspace contact on another team", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			contact := e.SeedContact(t, "Shared buyer", &e.Rep3)
			if tc.private {
				e.MakeCapturePrivate(t, "contact", contact, e.Rep3)
			}
			owner := e.As(e.Rep3, nil, principal.Permissions{
				RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
				Objects: map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			})
			if tc.share != "" {
				if _, err := identity.NewService(e.Pool).CreateRecordGrant(owner, identity.CreateGrantInput{
					RecordType: "contact", RecordID: contact, SubjectType: "user", SubjectID: e.Rep1, Access: tc.share,
				}); err != nil {
					t.Fatal(err)
				}
			}
			user := e.Rep1
			if tc.owner {
				user = e.Rep3
			}
			reader := e.As(user, nil, principal.Permissions{
				RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
				Objects: map[string]principal.ObjectGrant{"contact": {Read: true}, "activity": {Create: true, Read: true}},
			})
			row, err := e.Contacts.GetContact(reader, ids.From[ids.ContactKind](contact), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if row.Attachable == nil || *row.Attachable != tc.want {
				t.Fatalf("attachable = %v, want %v", row.Attachable, tc.want)
			}
			if row.Writable == nil || *row.Writable {
				t.Fatal("a reader without contact update rights was told it could edit")
			}

			listed, _, err := e.Contacts.ListContacts(reader, contacts.ListContactsInput{})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, listedContact := range listed {
				if ids.UUID(listedContact.Id) == contact {
					found = true
					if listedContact.Attachable == nil || *listedContact.Attachable != tc.want {
						t.Fatal("the list and single read disagree on attach rights")
					}
				}
			}
			if !found {
				t.Fatal("the readable contact is absent from the list")
			}

			page, err := contactRoomService(e).Assemble(reader, ids.From[ids.ContactKind](contact))
			if err != nil {
				t.Fatal(err)
			}
			if page.Moment == nil {
				t.Fatal("the contact has no Next bar answer")
			}
			action := page.Moment.RecommendedAction
			if action.Kind != crmcontracts.ContactMomentActionKindLogActivity {
				t.Fatalf("the empty contact offers %s, want logging", action.Kind)
			}
			blocked := action.State == crmcontracts.ContactMomentActionStateBlocked
			if blocked == tc.want {
				t.Fatal("the Next bar and the attach flag disagree")
			}
			err = database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
				return auth.EnsureAttachTarget(reader, tx, "contact", contact)
			})
			if tc.want && err != nil {
				t.Fatalf("the attach gate refused an attachable contact: %v", err)
			}
			if !tc.want && !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("the attach gate returned %v, want permission denied", err)
			}
		})
	}
}
