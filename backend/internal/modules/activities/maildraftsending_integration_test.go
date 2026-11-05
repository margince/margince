// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The files, reason for contact and moment a saved draft keeps, against a real
// database. A file the author cannot see is neither kept nor named.

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// seedDraftFile hangs a file off its own activity, which the rep sees until
// linkToContactOwnedBy hides it.
func (e *sendEnv) seedDraftFile(t *testing.T, filename string) (ids.UUID, ids.ActivityID) {
	t.Helper()
	parent := e.seedAnchor(t, "", "")
	file := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO attachment (id, entity_type, entity_id, filename, content_type, byte_size,
			storage_key, checksum, source, captured_by)
		VALUES ($1, 'activity', $2, $3, 'application/pdf', 2048, $4, 'sum', 'human', 'human:test')`,
		file, parent, filename, "draft-files/"+file.String()); err != nil {
		t.Fatalf("seeding the file: %v", err)
	}
	return file, parent
}

func (e *sendEnv) storedDraftFiles(t *testing.T, draft ids.UUID) []ids.UUID {
	t.Helper()
	var stored []ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT attachment_ids FROM mail_draft WHERE id = $1`, draft).Scan(&stored); err != nil {
		t.Fatalf("reading the draft's files: %v", err)
	}
	return stored
}

func attachedIDs(files resolvedFiles) []ids.UUID {
	out := make([]ids.UUID, 0, len(files.attachable))
	for _, file := range files.attachable {
		out = append(out, file.AttachmentID)
	}
	return out
}

func contactAnchor(t *testing.T, e *sendEnv) MailDraftAnchor {
	t.Helper()
	return MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
}

func TestASavedDraftReadsBackWithItsFilesReasonAndMoment(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeOwn)
	anchor := contactAnchor(t, e)
	file, _ := e.seedDraftFile(t, "quote.pdf")
	moment := &SendSchedule{At: draftClock.Add(26 * time.Hour), TZ: "Europe/Berlin"}
	content := typedDraft()
	content.AttachmentIDs = []ids.UUID{file, file}
	content.Context = commsauthz.CategoryPrecontractQuote
	content.Schedule = moment
	if _, err := draftStore(e).SaveMailDraft(ctx, anchor, content, nil); err != nil {
		t.Fatalf("saving: %v", err)
	}

	got, err := draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(got.Files.attachable) != 1 || got.Files.unavailable != 0 {
		t.Fatalf("read back files %+v, want the one file chosen twice, once", got.Files)
	}
	chip := got.Files.attachable[0]
	if chip.AttachmentID != file || chip.Filename != "quote.pdf" || chip.ContentType != "application/pdf" || chip.ByteSize != 2048 {
		t.Errorf("read back %+v, want quote.pdf as stored, so the composer draws its chip", chip)
	}
	if got.Content.Context != commsauthz.CategoryPrecontractQuote {
		t.Errorf("read back reason %q, want precontract_quote", got.Content.Context)
	}
	if got.Content.Schedule == nil || !got.Content.Schedule.At.Equal(moment.At) || got.Content.Schedule.TZ != moment.TZ {
		t.Errorf("read back moment %+v, want %+v", got.Content.Schedule, moment)
	}
}

// A file the author cannot see is left out of the row and the answer alike. It
// counts the same as one that never existed, so the count tells nothing.
func TestADraftKeepsNoFileItsAuthorCannotSee(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeOwn)
	visible, _ := e.seedDraftFile(t, "quote.pdf")
	hidden, hiddenParent := e.seedDraftFile(t, "their-contract.pdf")
	e.linkToContactOwnedBy(t, hiddenParent, e.other)
	content := typedDraft()
	content.AttachmentIDs = []ids.UUID{hidden, visible, ids.NewV7()}

	saved, err := draftStore(e).SaveMailDraft(ctx, contactAnchor(t, e), content, nil)
	if err != nil {
		t.Fatalf("saving with a file out of sight → %v, want the words kept", err)
	}
	if got := attachedIDs(saved.Files); !slices.Equal(got, []ids.UUID{visible}) || saved.Files.unavailable != 2 {
		t.Errorf("the save answered files %v with %d unavailable, want only the visible one and 2", got, saved.Files.unavailable)
	}
	if stored := e.storedDraftFiles(t, saved.ID); !slices.Equal(stored, []ids.UUID{visible}) {
		t.Errorf("the row keeps %v, want only the file its author can see", stored)
	}
	for _, file := range saved.Files.attachable {
		if file.Filename == "their-contract.pdf" {
			t.Error("the answer named a file its author cannot see")
		}
	}
}

// A file archived, or out of the author's sight, after the save is left out
// of the read rather than failing it.
func TestADraftReadLeavesOutAFileGoneSinceTheSave(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeOwn)
	anchor := contactAnchor(t, e)
	kept, _ := e.seedDraftFile(t, "kept.pdf")
	archived, _ := e.seedDraftFile(t, "archived.pdf")
	hidden, hiddenParent := e.seedDraftFile(t, "hidden.pdf")
	content := typedDraft()
	content.AttachmentIDs = []ids.UUID{kept, archived, hidden}
	if _, err := draftStore(e).SaveMailDraft(ctx, anchor, content, nil); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE attachment SET archived_at = now() WHERE id = $1`, archived); err != nil {
		t.Fatalf("archiving the file: %v", err)
	}
	e.linkToContactOwnedBy(t, hiddenParent, e.other)

	got, err := draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatalf("reading a draft whose files went → %v, want the draft without them", err)
	}
	if files := attachedIDs(got.Files); !slices.Equal(files, []ids.UUID{kept}) || got.Files.unavailable != 2 {
		t.Errorf("read back files %v with %d unavailable, want only kept.pdf and 2", files, got.Files.unavailable)
	}
}

// A save replaces the whole draft. A body without the sending fields clears
// them, as it clears a missing subject.
func TestASaveWithoutTheSendingFieldsClearsThem(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := contactAnchor(t, e)
	file, _ := e.seedDraftFile(t, "quote.pdf")
	content := typedDraft()
	content.AttachmentIDs = []ids.UUID{file}
	content.Context = commsauthz.CategoryMarketing
	content.Schedule = &SendSchedule{At: draftClock.Add(time.Hour), TZ: "UTC"}
	store := draftStore(e)
	saved, err := store.SaveMailDraft(ctx, anchor, content, nil)
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if _, err := store.SaveMailDraft(ctx, anchor, typedDraft(), &saved.Version); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	got, err := store.GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(got.Files.attachable) != 0 || got.Content.Context != "" || got.Content.Schedule != nil {
		t.Errorf("read back %+v and files %+v, want no files, no reason and no moment", got.Content, got.Files)
	}
	actions, images := e.draftTrail(t, saved.ID)
	if strings.Join(actions, ",") != "create,update" {
		t.Errorf("audit trail = %v, want create, update", actions)
	}
	for _, word := range []string{file.String(), "quote.pdf", string(commsauthz.CategoryMarketing)} {
		if strings.Contains(images, word) {
			t.Errorf("the draft's audit images quote %q; they carry the anchor and version only", word)
		}
	}
}

// The agent's writer readies its content through the composer's own path, so a
// file its human cannot see is left out there too.
func TestAnAgentsDraftKeepsNoFileItsHumanCannotSee(t *testing.T) {
	e := setupSend(t)
	hidden, hiddenParent := e.seedDraftFile(t, "their-contract.pdf")
	e.linkToContactOwnedBy(t, hiddenParent, e.other)
	content := agentDraft("With the contract")
	content.AttachmentIDs = []ids.UUID{hidden}

	saved, err := draftStore(e).SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeOwn), contactAnchor(t, e), content, nil)
	if err != nil {
		t.Fatalf("an agent's draft naming a hidden file → %v, want its words kept", err)
	}
	if len(saved.Files.attachable) != 0 || len(e.storedDraftFiles(t, saved.ID)) != 0 {
		t.Errorf("the agent's draft kept %v, want no file its human cannot see", saved.Files.attachable)
	}
}

// A new version of an agent's words keeps the files the draft holds. A route save
// replaces the whole draft, so one that names no files clears them.
func TestANewVersionOfAnAgentsWordsKeepsTheDraftsFiles(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	agent := e.asAgentFor(t, principal.RowScopeAll)
	kept, _ := e.seedDraftFile(t, "quote.pdf")
	first := agentDraft("First go")
	first.AttachmentIDs = []ids.UUID{kept}
	if _, err := store.SaveAgentMailDraft(agent, anchor, first, nil); err != nil {
		t.Fatal(err)
	}
	again, err := store.SaveAgentMailDraft(agent, anchor, agentDraft("Shorter"), nil)
	if err != nil {
		t.Fatalf("the new words: %v", err)
	}
	if !slices.Equal(e.storedDraftFiles(t, again.ID), []ids.UUID{kept}) || !slices.Equal(attachedIDs(again.Files), []ids.UUID{kept}) {
		t.Errorf("the new words left files %v, answered %v, want %s", e.storedDraftFiles(t, again.ID), attachedIDs(again.Files), kept)
	}
	routed, err := store.SaveMailDraft(agent, anchor, agentDraft("Shorter still"), &again.Version)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.storedDraftFiles(t, routed.ID); len(got) != 0 {
		t.Errorf("a route save naming no files left %v", got)
	}
}

// A file the draft holds that its author can no longer see stays held through new
// words, and no read shows it.
func TestAHeldFileNowHiddenStaysOutOfTheRead(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	agent := e.asAgentFor(t, principal.RowScopeOwn)
	kept, _ := e.seedDraftFile(t, "kept.pdf")
	hidden, hiddenParent := e.seedDraftFile(t, "hidden.pdf")
	first := agentDraft("First go")
	first.AttachmentIDs = []ids.UUID{kept, hidden}
	if _, err := store.SaveAgentMailDraft(agent, anchor, first, nil); err != nil {
		t.Fatal(err)
	}
	e.linkToContactOwnedBy(t, hiddenParent, e.other)
	again, err := store.SaveAgentMailDraft(agent, anchor, agentDraft("Shorter"), nil)
	if err != nil {
		t.Fatalf("the new words: %v", err)
	}
	read, err := store.GetMailDraft(agent, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if got := attachedIDs(read.Files); !slices.Equal(got, []ids.UUID{kept}) || !slices.Equal(attachedIDs(again.Files), []ids.UUID{kept}) {
		t.Errorf("the draft reads files %v and answered %v, want only %s", got, attachedIDs(again.Files), kept)
	}
}

// New words read the files the draft holds under the row's lock. So files another
// save put there while this one waited are the files the new version keeps.
func TestNewWordsKeepTheFilesASaveMeanwhileLeft(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	agent := e.asAgentFor(t, principal.RowScopeAll)
	first, _ := e.seedDraftFile(t, "first.pdf")
	newer, _ := e.seedDraftFile(t, "newer.pdf")
	draft := agentDraft("First go")
	draft.AttachmentIDs = []ids.UUID{first}
	saved, err := store.SaveAgentMailDraft(agent, anchor, draft, nil)
	if err != nil {
		t.Fatal(err)
	}
	saver, err := pgx.Connect(context.Background(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := saver.Close(context.Background()); err != nil {
			t.Errorf("closing the saver: %v", err)
		}
	})
	meanwhile, err := saver.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var pid int32
	if err := meanwhile.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := meanwhile.Exec(context.Background(), `UPDATE mail_draft SET attachment_ids = $2 WHERE id = $1`, saved.ID, []ids.UUID{newer}); err != nil {
		t.Fatal(err)
	}
	returned := make(chan struct{})
	var regenerated error
	go func() {
		defer close(returned)
		_, regenerated = store.SaveAgentMailDraft(agent, anchor, agentDraft("Shorter"), nil)
	}()
	testdb.AwaitQueuedBehind(t, e.owner, pid, 1, returned, "the new words")
	if err := meanwhile.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-returned
	if regenerated != nil {
		t.Fatal(regenerated)
	}
	if got := e.storedDraftFiles(t, saved.ID); !slices.Equal(got, []ids.UUID{newer}) {
		t.Errorf("the new words left files %v, want the newer save's %s", got, newer)
	}
}
