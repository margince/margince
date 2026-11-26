// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An address that moved from one contact to another keeps its older mail with
// the contact who wrote it. That mail neither settles nor prevents the new
// owner's notice duty.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const movedAddress = "shared.inbox@customer.test"

// movedAddressPair makes the former holder, whose address row is archived, and
// the contact who holds the address now and is owed a source review.
func movedAddressPair(t *testing.T, e *apptest.AppEnv) (former, holder string) {
	t.Helper()
	former = capturedUnknown(t, e, "Former Holder")
	withAddress(t, e, former, movedAddress)
	if _, err := e.Owner.Exec(context.Background(), `
		UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, former); err != nil {
		t.Fatalf("retiring the former holder's address: %v", err)
	}
	holder = capturedUnknown(t, e, "New Holder")
	withAddress(t, e, holder, movedAddress)
	return former, holder
}

// formerHoldersMail is mail capture filed under the former holder while the
// address was theirs: the sender row names them and carries the address.
func formerHoldersMail(t *testing.T, e *apptest.AppEnv, former string) ids.UUID {
	t.Helper()
	mail := capturedMailFrom(t, e, movedAddress, false)
	if _, err := e.Owner.Exec(context.Background(), `
		UPDATE activity_participant SET contact_id = $2 WHERE activity_id = $1 AND role = 'from'`,
		mail, former); err != nil {
		t.Fatalf("naming the former holder as the sender: %v", err)
	}
	return mail
}

func TestAMovedAddressSettlesOnlyTheFormerHoldersNotice(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	former, holder := movedAddressPair(t, e)
	apptest.DriveNoticeCase(t, e, former)
	apptest.DriveNoticeCase(t, e, holder)

	driveCapturedMail(t, e, formerHoldersMail(t, e, former))

	if _, state, _, _ := noticeCaseFor(t, e, holder); state != "source_review" {
		t.Errorf("the new holder's case is %q after the former holder's mail, want source_review: they did not write it", state)
	}
	if acquisitionKinds(t, e, holder)["subject_initiated"] {
		t.Error("the former holder's mail was recorded as the new holder writing to us")
	}
	if _, state, _, _ := noticeCaseFor(t, e, former); state != "exempt_with_reason" {
		t.Errorf("the former holder's case is %q after their own mail, want exempt_with_reason", state)
	}
}

func TestAMovedAddressStillOwesTheNewHolderANotice(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	former, holder := movedAddressPair(t, e)
	formerHoldersMail(t, e, former)

	apptest.DriveNoticeCase(t, e, holder)

	if _, _, _, found := noticeCaseFor(t, e, holder); !found {
		t.Error("the former holder's captured mail kept a notice case from opening for the new holder")
	}
}

// A sender row that carries only the moved address proves nobody, so it
// settles no one's duty.
func TestABareMovedAddressSettlesNoNotice(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	former, holder := movedAddressPair(t, e)
	apptest.DriveNoticeCase(t, e, holder)

	driveCapturedMail(t, e, capturedMailFrom(t, e, movedAddress, false))

	if _, state, _, _ := noticeCaseFor(t, e, holder); state != "source_review" {
		t.Errorf("the new holder's case is %q after mail from the bare moved address, want source_review", state)
	}
	for _, contact := range []string{former, holder} {
		if acquisitionKinds(t, e, contact)["subject_initiated"] {
			t.Errorf("mail from the bare moved address was recorded as %s writing to us", contact)
		}
	}
}
