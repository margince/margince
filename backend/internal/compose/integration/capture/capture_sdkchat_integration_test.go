// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A member-owned chat message from an extension unit follows its contact as mail
// does, and carries its files like mail. The message goes through the mapping
// of the unit door (extingress.ChatInput) and then through the function every
// chat network shares. What these cases prove holds for the unit's path.

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/extingress"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

const unitCapturedBy = "connector:ext:memberchat"

// unitSeatCtx is the connector principal a unit's ingest runs as: the unit's
// connector identity wearing the member's permissions.
func unitSeatCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: unitCapturedBy, UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// unitChat files one message of the neutral member-owned unit, on the contact
// the member chose, as the unit door maps it.
func unitChat(
	t *testing.T, e *integration.SearchEnv, blob blobstore.Store, key string, contact ids.UUID, files ...extension.InboundFile,
) ids.UUID {
	t.Helper()
	rec := extension.Record{
		System: "member-chat", Key: key, ThreadKey: "member-chat:thread-1",
		Activity: extension.ActivityFields{
			Kind: extension.ActivityKindMessage, ChannelProvider: "telegram", Body: "the unit's chat",
			OccurredAt: clocktest.Now(t).UTC(), Direction: extension.DirectionInbound,
		},
		Counterparty: extension.Counterparty{
			DisplayName: "Buyer", ChannelIdentity: extension.ChannelIdentity{Provider: "telegram", ChannelUserID: "770" + key},
		},
		ChosenSubject: &extension.ChosenSubject{Kind: extension.SubjectContact, ID: contact.String()},
		Parts:         files,
		Raw:           []byte(`{"id":"` + key + `"}`),
	}
	binding, msg, err := extingress.ChatInput(extingress.ChatSource{
		CapturedBy: unitCapturedBy, SourceSystem: "ext:memberchat:member-chat", Member: e.Rep1,
		Source: extension.IngressSource{System: "member-chat", PinnedPosture: extension.PostureClassified}, Now: clocktest.Now(t).UTC(),
	}, rec)
	if err != nil {
		t.Fatalf("mapping the unit's record: %v", err)
	}
	ref, err := capturemod.FileChatMessage(unitSeatCtx(e), compose.NewCaptureSink(e.Pool, compose.CaptureConfig{Blob: blob}), binding, msg)
	if err != nil {
		t.Fatalf("filing the unit's chat message: %v", err)
	}
	return ref.ID
}

// The product owner's acceptance case. One contact has a mail and a unit chat
// message, both held for the member's classified mailbox and pin. Each change of
// the contact's sharing, through the real publish and patch paths, changes who
// reads both in the same way.
func TestAUnitsChatMessageFollowsItsContactsSharingExactlyAsMailDoes(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE capture_connection SET mail_posture = 'classified' WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("setting the mailbox posture: %v", err)
	}
	const counterparty = "buyer@kunde.example"
	env.syncSent(t, map[string]bool{"sdk-1@kunde.example": true},
		emailSaying(counterparty, "sdk-1@kunde.example", "", "danke für das Angebot"))
	env.sync(t, email(counterparty, "Buyer", captureOwner, "sdk-1r@kunde.example", "sdk-1@kunde.example"))
	mail := activityIDOf(t, e, "sdk-1@kunde.example")
	var contactRaw ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.id FROM contact p JOIN contact_email pe ON pe.contact_id = p.id WHERE pe.email = $1`, counterparty).Scan(&contactRaw)
	}); err != nil {
		t.Fatalf("finding the contact: %v", err)
	}
	chat := unitChat(t, e, nil, "1", contactRaw)

	same := func(step string) []ids.UUID {
		t.Helper()
		fromMail, fromChat := readersOf(t, e, mail), readersOf(t, e, chat)
		if !slices.Equal(fromMail, fromChat) {
			t.Fatalf("%s: mail is readable by %v and the unit's chat message by %v, want the same seats", step, fromMail, fromChat)
		}
		return fromMail
	}
	if readers := same("a private contact"); !slices.Equal(readers, []ids.UUID{e.Rep1}) {
		t.Fatalf("a private contact's correspondence is readable by %v, want only its owner %v", readers, e.Rep1)
	}

	store := contacts.NewStore(e.DB()).WithSenderStamp(activities.StampProvenSendersTx)
	ownerCtx := func() context.Context {
		ctx := principal.WithWorkspaceID(context.Background(), e.WS)
		ctx = principal.WithCorrelationID(ctx, ids.NewV7())
		return principal.WithActor(ctx, principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull,
			Permissions: principal.Permissions{
				RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
				Objects: map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}, "activity": {Read: true, Update: true}},
			},
		})
	}
	contact := ids.From[ids.ContactKind](contactRaw)
	if err := store.PromoteOwnCapturedContact(ownerCtx(), contact); err != nil {
		t.Fatalf("publishing the contact: %v", err)
	}
	if readers := same("a published contact"); len(readers) != 1 {
		t.Fatalf("classified correspondence of a published contact is readable by %v, want it held from the colleague until judged", readers)
	}
	private := "owner"
	if _, err := store.UpdateContact(ownerCtx(), contact, contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private again: %v", err)
	}
	same("a contact made private again")
}

// One eligible file on a unit's message reaches the contact's History. A retry
// keeps one copy. Erasing the contact removes the file rows, queues the object
// for deletion, and removes the original even though its payload names nothing about the contact.
func TestAFileOnAUnitsMessageIsKeptOnceAndErasedWithItsContact(t *testing.T) {
	blob := blobstore.NewMemory()
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	e := env.e
	contact := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO contact (id, full_name, source, captured_by, owner_id) VALUES ($1, 'Erasure Subject', 'manual', 'human:x', $2)`, contact, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	pdf := extension.InboundFile{Ordinal: 0, Filename: "offer.pdf", Body: append([]byte("%PDF-1.4 "), bytes.Repeat([]byte("x"), 64)...)}

	chat := unitChat(t, e, blob, "file-1", contact, pdf)
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), query, args...).Scan(&n)
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return n
	}
	if got := count(`SELECT count(*) FROM attachment WHERE activity_id = $1 AND filename = 'offer.pdf'`, chat); got != 1 {
		t.Fatalf("%d files on the message, want the one eligible file", got)
	}
	if _, _, err := blob.Get(context.Background(), blobKeyOf(t, e, chat)); err != nil {
		t.Fatalf("the file's bytes are not in the object store: %v", err)
	}

	if again := unitChat(t, e, blob, "file-1", contact, pdf); again != chat {
		t.Fatalf("a retry filed %s, want the same message %s", again, chat)
	}
	if got := count(`SELECT count(*) FROM attachment WHERE activity_id = $1`, chat); got != 1 {
		t.Fatalf("%d files after a retry, want still one copy", got)
	}

	key := blobKeyOf(t, e, chat)
	raw := count(`SELECT count(*) FROM raw_capture WHERE source_id = 'file-1'`)
	if raw == 0 {
		t.Fatal("the message left no evidence to erase, so the erasure check would prove nothing")
	}
	if err := compose.ComposedEraser(e.Pool, blob).EraseContact(erasureCtx(e), contact, "subject request"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	if got := count(`SELECT count(*) FROM attachment WHERE activity_id = $1`, chat); got != 0 {
		t.Errorf("%d files survive the erasure", got)
	}
	if got := count(`SELECT count(*) FROM raw_capture WHERE source_id = 'file-1'`); got != 0 {
		t.Errorf("%d evidence rows survive the erasure", got)
	}
	if got := count(`SELECT count(*) FROM stored_object_intent WHERE storage_key = $1`, key); got != 1 {
		t.Errorf("%d deletion entries for the file's object, want the one the reap works from", got)
	}
}

// blobKeyOf is the object key the message's file is stored under.
func blobKeyOf(t *testing.T, e *integration.SearchEnv, activity ids.UUID) string {
	t.Helper()
	var key string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT storage_key FROM attachment WHERE activity_id = $1`, activity).Scan(&key)
	}); err != nil {
		t.Fatalf("reading the file's storage key: %v", err)
	}
	return key
}

// erasureCtx is an administrator who may delete the subject's records.
func erasureCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull,
		Permissions: integration.AdminPerms,
	})
}
