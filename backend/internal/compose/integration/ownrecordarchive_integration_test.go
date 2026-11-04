// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownRecordArchivePerms is the rep's seeded shape on contacts and companies:
// write without delete, team scope, so a teammate's record is writable and a
// refusal below comes from the archive rule.
func ownRecordArchivePerms() principal.Permissions {
	perms := ownArchivePerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects[objCompany] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return perms
}

// The owner of a contact archives it without the delete grant; a teammate's
// contact, writable all the same, stays a manager's to archive. The read says
// which before anyone presses Archive.
func TestTheOwnerOfAContactArchivesIt(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownRecordArchivePerms())
	mine := ids.From[ids.ContactKind](e.SeedContact(t, "Mine Example", &e.Rep1))
	theirs := ids.From[ids.ContactKind](e.SeedContact(t, "Theirs Example", &e.Rep2))

	read, err := e.Contacts.GetContact(rep, theirs, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !*read.Writable || *read.Archivable {
		t.Errorf("teammate's contact: writable=%v archivable=%v, want true/false", *read.Writable, *read.Archivable)
	}
	if _, err := e.Contacts.ArchiveContact(rep, theirs, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a teammate's contact → %v, want ErrPermissionDenied", err)
	}
	if _, err := e.Contacts.PreviewContactArchive(rep, theirs, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("previewing the archive of a teammate's contact → %v, want ErrPermissionDenied", err)
	}
	if read, err = e.Contacts.GetContact(rep, mine, storekit.LiveOnly); err != nil || !*read.Archivable {
		t.Fatalf("my own contact: archivable=%v err=%v, want true", read.Archivable, err)
	}
	if _, err := e.Contacts.ArchiveContact(rep, mine, nil); err != nil {
		t.Fatalf("archiving my own contact → %v, want allowed", err)
	}
	archiveWroteTheShape(t, e, mine.UUID, "contact.archived")
}

// The same rule on a company.
func TestTheOwnerOfACompanyArchivesIt(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownRecordArchivePerms())
	mine := ids.From[ids.CompanyKind](e.SeedCompany(t, "Mine GmbH", &e.Rep1))
	theirs := ids.From[ids.CompanyKind](e.SeedCompany(t, "Theirs GmbH", &e.Rep2))

	read, err := e.Contacts.GetCompany(rep, theirs, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !*read.Writable || *read.Archivable {
		t.Errorf("teammate's company: writable=%v archivable=%v, want true/false", *read.Writable, *read.Archivable)
	}
	if _, err := e.Contacts.ArchiveCompany(rep, theirs, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a teammate's company → %v, want ErrPermissionDenied", err)
	}
	if read, err = e.Contacts.GetCompany(rep, mine, storekit.LiveOnly); err != nil || !*read.Archivable {
		t.Fatalf("my own company: archivable=%v err=%v, want true", read.Archivable, err)
	}
	if _, err := e.Contacts.ArchiveCompany(rep, mine, nil); err != nil {
		t.Fatalf("archiving my own company → %v, want allowed", err)
	}
	archiveWroteTheShape(t, e, mine.UUID, "company.archived")
}

// A rep who names themselves owner of a teammate's contact has not been given
// it, so it stays a manager's to archive.
func TestAContactTheRepNamedThemselvesOwnerOfStaysTheManagersToArchive(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownRecordArchivePerms())
	me := ids.From[ids.UserKind](e.Rep1)
	taken := ids.From[ids.ContactKind](e.SeedContact(t, "Taken Example", &e.Rep2))
	if _, err := e.Contacts.UpdateContact(rep, taken, contacts.UpdateContactInput{OwnerID: &me}); err != nil {
		t.Fatalf("a teammate naming themselves owner: %v", err)
	}
	if _, err := e.Contacts.ArchiveContact(rep, taken, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a contact the rep took over themselves → %v, want ErrPermissionDenied", err)
	}
}
