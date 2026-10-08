// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type restoreFixture struct {
	e     *Env
	store *collections.Store
	rep   context.Context
}

func newRestoreFixture(t *testing.T) restoreFixture {
	e := Setup(t)
	return restoreFixture{e: e, store: collections.NewStore(e.DB()), rep: e.As(e.Rep1, []ids.UUID{e.Team1}, restorePerms())}
}

func restorePerms() principal.Permissions {
	return withGrant(listPerms(), "tag", principal.ObjectGrant{Read: true})
}

func withGrant(p principal.Permissions, object string, grant principal.ObjectGrant) principal.Permissions {
	p.Objects = maps.Clone(p.Objects)
	p.Objects[object] = grant
	return p
}

// otherTenant asks through a store bound to its own workspace, as a tenant would.
func (f restoreFixture) otherTenant(t *testing.T) (context.Context, *collections.Store) {
	t.Helper()
	ws, user := ids.NewV7(), ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(), `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding the second workspace: %v", err)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), ws), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, Permissions: AdminPerms,
	})
	return ctx, collections.NewStore(f.e.DBFor(ws))
}

func (f restoreFixture) tag(t *testing.T, name string) ids.TagID {
	t.Helper()
	tag, err := f.store.NewTag(f.e.Admin(), name, "")
	if err != nil {
		t.Fatalf("seeding the tag: %v", err)
	}
	return ids.From[ids.TagKind](tag.TagID)
}

func (f restoreFixture) taggedByRep(t *testing.T, tag ids.TagID, name string) ids.UUID {
	t.Helper()
	contact := f.e.SeedContact(t, name, &f.e.Rep1)
	if _, err := f.store.ApplyTag(f.rep, tag, "contact", contact); err != nil {
		t.Fatalf("applying the tag: %v", err)
	}
	return contact
}

func (f restoreFixture) removeTag(ctx context.Context, t *testing.T, tag ids.TagID, contact ids.UUID) ids.UUID {
	t.Helper()
	removal, err := f.store.RemoveTag(ctx, tag, "contact", contact)
	if err != nil || removal == ids.Nil {
		t.Fatalf("removing the tag → %v, %v; want the removal's audit id", removal, err)
	}
	return removal
}

func (f restoreFixture) tagProvenance(t *testing.T, tag ids.TagID, contact ids.UUID) string {
	t.Helper()
	return f.e.WsScalar(t, `SELECT concat_ws('|', id, created_at, assigned_by, assigned_by_kind, assigned_at) FROM taggable
		WHERE tag_id = $1 AND entity_type = 'contact' AND entity_id = $2`, tag, contact)
}

func TestARemovedTagComesBackAsItWasAssigned(t *testing.T) {
	f := newRestoreFixture(t)
	tag := f.tag(t, "Undo Me")
	contact := f.taggedByRep(t, tag, "Tagged By Rep")
	assigned := f.tagProvenance(t, tag, contact)

	removal := f.removeTag(f.e.Admin(), t, tag, contact)
	restored, err := f.store.RestoreTagRemoval(f.e.Admin(), tag, removal)
	if err != nil {
		t.Fatalf("restoring the removal: %v", err)
	}
	if restored.EntityID != contact || restored.TagID != tag {
		t.Fatalf("the restore answered %+v, want the tagging of %s", restored, contact)
	}
	if back := f.tagProvenance(t, tag, contact); back != assigned {
		t.Fatalf("the tagging came back as %q, want the assignment it had: %q", back, assigned)
	}
	undid := f.e.WsScalar(t, `SELECT evidence ->> $2 FROM audit_log
		WHERE entity_type = 'tag' AND entity_id = $1 AND after ? 'applied' ORDER BY occurred_at DESC LIMIT 1`,
		tag, storekit.EvidenceKeyUndidAuditLog)
	if undid != removal.String() {
		t.Fatalf("the restore's audit row names %q as what it undid, want %s", undid, removal)
	}
	if _, err := f.store.RestoreTagRemoval(f.e.Admin(), tag, removal); !errors.Is(err, collections.ErrRemovalMovedOn) {
		t.Fatalf("restoring the same removal twice → %v, want ErrRemovalMovedOn", err)
	}

	untagged := f.e.SeedContact(t, "Never Tagged", &f.e.Rep1)
	if removal, err := f.store.RemoveTag(f.rep, tag, "contact", untagged); err != nil || removal != ids.Nil {
		t.Fatalf("removing a tagging that is not there → %v, %v; want no removal and no error", removal, err)
	}
}

func TestATagRestoreRefusesATaggingThatMovedOn(t *testing.T) {
	f := newRestoreFixture(t)
	tag := f.tag(t, "Moving On")
	contact := f.taggedByRep(t, tag, "Retagged")

	first := f.removeTag(f.e.Admin(), t, tag, contact)
	if _, err := f.store.ApplyTag(f.e.Admin(), tag, "contact", contact); err != nil {
		t.Fatalf("applying the tag again: %v", err)
	}
	if _, err := f.store.RestoreTagRemoval(f.e.Admin(), tag, first); !errors.Is(err, collections.ErrRemovalMovedOn) {
		t.Fatalf("restoring over a tag applied again → %v, want ErrRemovalMovedOn", err)
	}
	second := f.removeTag(f.e.Admin(), t, tag, contact)
	if _, err := f.store.RestoreTagRemoval(f.e.Admin(), tag, first); !errors.Is(err, collections.ErrRemovalMovedOn) {
		t.Fatalf("restoring a removal a later one superseded → %v, want ErrRemovalMovedOn", err)
	}
	if _, err := f.store.RestoreTagRemoval(f.e.Admin(), tag, second); err != nil {
		t.Fatalf("restoring the latest removal: %v", err)
	}

	retired := f.tag(t, "Retired Since")
	other := f.taggedByRep(t, retired, "Tagged With A Retired Word")
	removal := f.removeTag(f.e.Admin(), t, retired, other)
	if _, err := f.store.ArchiveTag(f.e.Admin(), retired); err != nil {
		t.Fatalf("archiving the tag: %v", err)
	}
	if _, err := f.store.RestoreTagRemoval(f.e.Admin(), retired, removal); !errors.Is(err, collections.ErrTagRetiredRestore) {
		t.Fatalf("restoring a tagging of an archived tag → %v, want ErrTagRetiredRestore", err)
	}
}

func TestATagRestoreHidesARemovalThatIsNotTheCallers(t *testing.T) {
	f := newRestoreFixture(t)
	tag, otherTag := f.tag(t, "Whose Removal"), f.tag(t, "Another Word")
	contact := f.taggedByRep(t, tag, "Removed By Admin")
	byAdmin := f.removeTag(f.e.Admin(), t, tag, contact)

	if _, err := f.store.RestoreTagRemoval(f.rep, tag, byAdmin); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring somebody else's removal → %v, want ErrNotFound", err)
	}
	if _, err := f.store.RestoreTagRemoval(f.e.Admin(), otherTag, byAdmin); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring a removal through another tag → %v, want ErrNotFound", err)
	}
	tenant, store := f.otherTenant(t)
	if _, err := store.RestoreTagRemoval(tenant, tag, byAdmin); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring from another workspace → %v, want ErrNotFound", err)
	}

	archived := f.taggedByRep(t, tag, "Archived Since")
	byRep := f.removeTag(f.rep, t, tag, archived)
	if _, err := f.e.Contacts.ArchiveContact(f.e.Admin(), ContactIDOf(archived), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := f.store.RestoreTagRemoval(f.rep, tag, byRep); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring onto an archived record → %v, want ErrNotFound", err)
	}

	kept := f.taggedByRep(t, tag, "Rep Lost The Grant")
	removal := f.removeTag(f.rep, t, tag, kept)
	readOnly := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, withGrant(restorePerms(), "contact", principal.ObjectGrant{Read: true}))
	if _, err := f.store.RestoreTagRemoval(readOnly, tag, removal); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("restoring without the record's update grant → %v, want ErrPermissionDenied", err)
	}
}

const memberNote = "met at the fair"

// shortlistByRep is shared workspace-wide so the admin can change it too.
func (f restoreFixture) shortlistByRep(t *testing.T, name string) (ids.ListID, ids.UUID) {
	t.Helper()
	list, err := f.store.CreateList(f.rep, collections.CreateListInput{Name: name, EntityType: "contact", Sharing: "workspace"})
	if err != nil {
		t.Fatalf("creating the Shortlist: %v", err)
	}
	contact := f.e.SeedContact(t, name+" member", &f.e.Rep1)
	note := memberNote
	if _, err := f.store.AddMember(f.rep, list.ID, f.member(contact, &note)); err != nil {
		t.Fatalf("adding the member: %v", err)
	}
	return list.ID, contact
}

func (f restoreFixture) member(contact ids.UUID, note *string) collections.MemberChange {
	return collections.MemberChange{EntityType: "contact", EntityID: contact, Note: note, Reason: collections.ReasonChosen}
}

func (f restoreFixture) removeMember(ctx context.Context, t *testing.T, list ids.ListID, contact ids.UUID) ids.UUID {
	t.Helper()
	removed, err := f.store.RemoveMember(ctx, list, f.member(contact, nil))
	if err != nil {
		t.Fatalf("removing the member: %v", err)
	}
	return removed.AuditID
}

func (f restoreFixture) memberProvenance(t *testing.T, list ids.ListID, contact ids.UUID) string {
	t.Helper()
	return f.e.WsScalar(t, `SELECT concat_ws('|', id, added_by, created_at, note) FROM list_member
		WHERE list_id = $1 AND entity_type = 'contact' AND entity_id = $2`, list, contact)
}

func TestARemovedMemberComesBackAsItWasAdded(t *testing.T) {
	f := newRestoreFixture(t)
	list, contact := f.shortlistByRep(t, "Fair Leads")
	added := f.memberProvenance(t, list, contact)

	removal := f.removeMember(f.e.Admin(), t, list, contact)
	restored, err := f.store.RestoreMemberRemoval(f.e.Admin(), list, removal)
	if err != nil {
		t.Fatalf("restoring the removal: %v", err)
	}
	if restored.EntityID != contact || restored.AddedBy != "human:"+f.e.Rep1.String() {
		t.Fatalf("the restore answered %+v, want the member Rep1 added", restored)
	}
	if back := f.memberProvenance(t, list, contact); back != added {
		t.Fatalf("the member came back as %q, want what it was added as: %q", back, added)
	}
	event := f.e.WsScalar(t, `SELECT concat_ws('|', action, reason, actor, coalesce(note, '<no note>')) FROM list_member_event
		WHERE list_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1`, list)
	if want := "added|chosen|human:" + f.e.AdminUser.String() + "|<no note>"; event != want {
		t.Fatalf("the restore's history entry = %q, want %q", event, want)
	}
	history, _, err := f.store.History(f.e.Admin(), list, 10, "")
	if err != nil {
		t.Fatalf("reading the list's history: %v", err)
	}
	for _, entry := range history {
		if entry.Note != nil && *entry.Note == memberNote && entry.Actor != "human:"+f.e.Rep1.String() {
			t.Fatalf("history shows Rep1's note as %s's words: %+v", entry.Actor, entry)
		}
	}
	outbox := f.e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id = (o.envelope -> 'trace' ->> 'audit_log_id')::uuid
		WHERE o.envelope ->> 'type' = 'list.member_added' AND a.entity_id = $1 AND a.evidence ->> $2 = $3`,
		list, storekit.EvidenceKeyUndidAuditLog, removal.String())
	if outbox != 1 {
		t.Fatalf("%d outbox events link to the restore's audit row, want 1", outbox)
	}
	if _, err := f.store.RestoreMemberRemoval(f.e.Admin(), list, removal); !errors.Is(err, collections.ErrRemovalMovedOn) {
		t.Fatalf("restoring the same removal twice → %v, want ErrRemovalMovedOn", err)
	}
}

func TestAMemberRestoreRefusesAMembershipThatMovedOn(t *testing.T) {
	f := newRestoreFixture(t)
	list, contact := f.shortlistByRep(t, "Moving Members")

	first := f.removeMember(f.rep, t, list, contact)
	if _, err := f.store.AddMember(f.rep, list, f.member(contact, nil)); err != nil {
		t.Fatalf("adding the record again: %v", err)
	}
	if _, err := f.store.RestoreMemberRemoval(f.rep, list, first); !errors.Is(err, collections.ErrRemovalMovedOn) {
		t.Fatalf("restoring over a record added again → %v, want ErrRemovalMovedOn", err)
	}
	second := f.removeMember(f.rep, t, list, contact)
	if _, err := f.store.RestoreMemberRemoval(f.rep, list, first); !errors.Is(err, collections.ErrRemovalMovedOn) {
		t.Fatalf("restoring a removal a later one superseded → %v, want ErrRemovalMovedOn", err)
	}
	if _, err := f.store.RestoreMemberRemoval(f.rep, list, second); err != nil {
		t.Fatalf("restoring the latest removal: %v", err)
	}

	archived, member := f.shortlistByRep(t, "Archived Since")
	removal := f.removeMember(f.rep, t, archived, member)
	if _, err := f.store.ArchiveList(f.rep, archived); err != nil {
		t.Fatalf("archiving the list: %v", err)
	}
	if _, err := f.store.RestoreMemberRemoval(f.rep, archived, removal); !errors.Is(err, collections.ErrListArchived) {
		t.Fatalf("restoring onto an archived list → %v, want ErrListArchived", err)
	}
}

func TestAMemberRestoreHidesARemovalThatIsNotTheCallers(t *testing.T) {
	f := newRestoreFixture(t)
	list, contact := f.shortlistByRep(t, "Whose Removal")
	otherList, _ := f.shortlistByRep(t, "Another List")
	byAdmin := f.removeMember(f.e.Admin(), t, list, contact)

	if _, err := f.store.RestoreMemberRemoval(f.rep, list, byAdmin); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring somebody else's removal → %v, want ErrNotFound", err)
	}
	if _, err := f.store.RestoreMemberRemoval(f.e.Admin(), otherList, byAdmin); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring a removal through another list → %v, want ErrNotFound", err)
	}
	tenant, store := f.otherTenant(t)
	if _, err := store.RestoreMemberRemoval(tenant, list, byAdmin); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring from another workspace → %v, want ErrNotFound", err)
	}

	gone, member := f.shortlistByRep(t, "Member Archived Since")
	byRep := f.removeMember(f.rep, t, gone, member)
	if _, err := f.e.Contacts.ArchiveContact(f.e.Admin(), ContactIDOf(member), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := f.store.RestoreMemberRemoval(f.rep, gone, byRep); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("restoring an archived record → %v, want ErrNotFound", err)
	}

	kept, held := f.shortlistByRep(t, "Rep Lost The Grant")
	removal := f.removeMember(f.rep, t, kept, held)
	readOnly := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, withGrant(restorePerms(), "list", principal.ObjectGrant{Read: true}))
	if _, err := f.store.RestoreMemberRemoval(readOnly, kept, removal); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("restoring without the list update grant → %v, want ErrPermissionDenied", err)
	}
}

func TestAnErasedMembersNoteLeavesNoCopyAndItsRemovalCannotComeBack(t *testing.T) {
	f := newRestoreFixture(t)
	list, contact := f.shortlistByRep(t, "Erased Subject")
	removal := f.removeMember(f.e.Admin(), t, list, contact)
	const copies = `SELECT (SELECT count(*) FROM list_member WHERE note = $1)
		+ (SELECT count(*) FROM list_member_event WHERE note = $1 OR member_note = $1)
		+ (SELECT count(*) FROM audit_log WHERE concat(before::text, after::text, evidence::text) LIKE '%' || $1 || '%')`
	if kept := f.e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE member_note = $1`, memberNote); kept != 1 {
		t.Fatalf("the removal kept the note on %d event rows, want 1", kept)
	}

	if err := privacy.NewEraser(f.e.DB()).EraseContact(f.e.Admin(), contact, "subject request"); err != nil {
		t.Fatalf("erasing the member: %v", err)
	}
	if held := f.e.WsCount(t, copies, memberNote); held != 0 {
		t.Fatalf("%d rows still hold the erased member's note, want none", held)
	}
	if _, err := f.store.RestoreMemberRemoval(f.e.Admin(), list, removal); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("restoring a removal whose subject was erased → %v, want ErrNotFound", err)
	}
}
