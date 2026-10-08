// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// Who receives a file's events.
//
// A file is scoped by the record it hangs off and by nothing of its own: there
// is no `attachment` object grant and no owner column, so the subject is
// redirected to its parent and the parent's own answer stands. These prove the
// redirect reaches that answer in both directions, a subscriber who may read
// the parent receives the file's events, and one who may not receives nothing.
//
// Without a probe at all these events resolve to nothing and are silently
// fail-closed-denied, which looks like a subscription nobody made.
//
// The two positive cases are the ones that hold the redirect: both go red if it
// is removed. The two negative cases pass with or without it, a missing case
// denies as well, and they are here for the other direction, where the probe
// is wrong by granting rather than by withholding.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedFileOnCompany hangs a file off the company the fixture already seeds.
func (e *contractVisEnv) seedFileOnCompany(t *testing.T) ids.UUID {
	t.Helper()
	attachment := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO attachment (id, entity_type, entity_id, filename, byte_size,
			storage_key, checksum, source, captured_by)
		VALUES ($1, 'company', $2, 'terms.pdf', 9, $3, 'sha256:x', 'upload', 'human:test')`,
		attachment, e.company, "k/"+attachment.String()); err != nil {
		t.Fatalf("seeding the file: %v", err)
	}
	return attachment
}

func TestAFileReachesWhoeverMayReadTheRecordItHangsOff(t *testing.T) {
	e := setupContractVis(t)
	attachment := e.seedFileOnCompany(t)

	visible, err := e.store.entityVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"company": {Read: true}}),
		"attachment.created", "attachment", attachment,
	)
	if err != nil {
		t.Fatalf("probing the file's visibility: %v", err)
	}
	if !visible {
		t.Error("a subscriber who may read the company receives none of its files' events")
	}
}

// The grant the file borrows. There is no attachment.read to hold instead.
func TestAFileReachesNobodyWhoCannotReadThatRecord(t *testing.T) {
	e := setupContractVis(t)
	attachment := e.seedFileOnCompany(t)

	visible, err := e.store.entityVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"deal": {Read: true}}),
		"attachment.created", "attachment", attachment,
	)
	if err != nil {
		t.Fatalf("probing the file's visibility: %v", err)
	}
	if visible {
		t.Error("a subscriber with no read on the company received that company's file events")
	}
}

// An ARCHIVED file still announces its own archival. The probe
// does not filter the file's archived_at: attachment.archived is emitted after
// the column is set, so requiring a live row would decline to deliver the very
// event saying the file is gone, silently, which reads as nobody subscribing.
func TestAnArchivedFileStillAnnouncesItsArchival(t *testing.T) {
	e := setupContractVis(t)
	attachment := e.seedFileOnCompany(t)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE attachment SET archived_at = now() WHERE id = $1`, attachment); err != nil {
		t.Fatalf("archiving the file: %v", err)
	}

	visible, err := e.store.entityVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"company": {Read: true}}),
		"attachment.archived", "attachment", attachment,
	)
	if err != nil {
		t.Fatalf("probing the archived file's visibility: %v", err)
	}
	if !visible {
		t.Error("the event saying a file was archived reaches nobody, because the file is archived")
	}
}

// A file whose row is gone resolves to nothing rather than to everyone.
func TestAFileThatIsNotThereReachesNobody(t *testing.T) {
	e := setupContractVis(t)

	visible, err := e.store.entityVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"company": {Read: true}}),
		"attachment.created", "attachment", ids.NewV7(),
	)
	if err != nil {
		t.Fatalf("probing a missing file: %v", err)
	}
	if visible {
		t.Error("an attachment id matching no row resolved as visible")
	}
}

// A file on an ACTIVITY, the capture case, and the one parent kind whose
// visibility is its own predicate rather than the shared one. A mail's
// attachment is filed on the activity, so this is the common shape in
// production and the only arm that reaches EnsureActivityContentVisible.
func TestAFileOnAnActivityFollowsThatActivitysContentVisibility(t *testing.T) {
	e := setupContractVis(t)
	attachment := e.seedFileOnActivity(t)

	visible, err := e.store.entityVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"activity": {Read: true}}),
		"attachment.created", "attachment", attachment,
	)
	if err != nil {
		t.Fatalf("probing the file's visibility: %v", err)
	}
	if !visible {
		t.Error("a subscriber who may read the mail receives nothing about the file that arrived on it")
	}
}

// And the same file reaches nobody without that read.
func TestAFileOnAnActivityReachesNobodyWithoutActivityRead(t *testing.T) {
	e := setupContractVis(t)
	attachment := e.seedFileOnActivity(t)

	visible, err := e.store.entityVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"company": {Read: true}}),
		"attachment.created", "attachment", attachment,
	)
	if err != nil {
		t.Fatalf("probing the file's visibility: %v", err)
	}
	if visible {
		t.Error("a subscriber with no activity.read received a mail attachment's events")
	}
}

// seedFileOnActivity logs an activity the fixture's seat owns and files on it.
func (e *contractVisEnv) seedFileOnActivity(t *testing.T) ids.UUID {
	t.Helper()
	ctx := context.Background()
	activity, attachment := ids.NewV7(), ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: terms', 'inbound', 'manual', $2)`,
		activity, "human:"+e.user.String()); err != nil {
		t.Fatalf("seeding the activity: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO attachment (id, entity_type, entity_id, filename, byte_size,
			storage_key, checksum, source, captured_by, activity_id)
		VALUES ($1, 'activity', $2, 'terms.pdf', 9, $3, 'sha256:y', 'email_capture', 'connector:imap', $2)`,
		attachment, activity, "k/"+attachment.String()); err != nil {
		t.Fatalf("seeding the file on the activity: %v", err)
	}
	return attachment
}
