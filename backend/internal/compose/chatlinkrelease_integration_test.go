// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unqueuedChatJobs stands in for the job queue; a release queues only a flush.
type unqueuedChatJobs struct{}

func (unqueuedChatJobs) EnqueueFlushTx(context.Context, pgx.Tx, ids.UUID) error { return nil }

func (unqueuedChatJobs) EnqueuePullTx(context.Context, pgx.Tx, ids.UUID) error { return nil }

// chatReleaseEnv is one deal linked to a channel, the deals store compose
// builds, and the cg:chat-links consumer over the chat links store.
type chatReleaseEnv struct {
	e          *integration.Env
	deals      *deals.Store
	sync       *chatsync.Sync
	deal, link ids.UUID
}

func setupChatRelease(t *testing.T) *chatReleaseEnv {
	t.Helper()
	e := integration.Setup(t)
	f := &chatReleaseEnv{e: e, deals: deals.NewStore(e.DB(), DealsInstallation()), deal: ids.NewV7(), link: ids.NewV7()}
	pipeline, stage, connection := ids.NewV7(), ids.NewV7(), ids.NewV7()
	f.exec(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Chat release', false, 91)`, pipeline)
	f.exec(t, `INSERT INTO stage (id, pipeline_id, name, position) VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	f.exec(t, `INSERT INTO deal (id, name, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Acme renewal', $2, $3, $4, 'manual', 'system')`, f.deal, e.Rep1, pipeline, stage)
	f.exec(t, `INSERT INTO channel_connection (id, provider, channel_id, channel_label, address, credential_ref, status, connected_by)
		VALUES ($1, 'dispact', 'team', 'Margince', 'https://chat.test', 'ref', 'connected', $2)`, connection, e.Rep1)
	f.exec(t, `INSERT INTO chat_link (id, connection_id, provider, channel_slug, channel_name, deal_id, linked_by)
		VALUES ($1, $2, 'dispact', 'acme', 'Acme', $3, $4)`, f.link, connection, f.deal, e.Rep1)
	f.sync = chatsync.New(e.Pool, e.DB(), chatlinks.NewStore(e.DB(), nil, unqueuedChatJobs{}, chatsync.Viewers))
	return f
}

func (f *chatReleaseEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	}); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f *chatReleaseEnv) archive(t *testing.T) {
	t.Helper()
	if _, err := f.deals.ArchiveDeal(f.e.Admin(), ids.From[ids.DealKind](f.deal), nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
}

// restore brings the deal back as a bulk restore does.
func (f *chatReleaseEnv) restore(t *testing.T) {
	t.Helper()
	ctx := f.e.Admin()
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		_, err := f.deals.RestoreDealTx(ctx, tx, ids.From[ids.DealKind](f.deal), nil, unarchiveWith(nil))
		return err
	}); err != nil {
		t.Fatalf("restoring the deal: %v", err)
	}
}

// outboxed reads the deal's latest event of eventType as the bus carries it.
func (f *chatReleaseEnv) outboxed(t *testing.T, eventType string) events.Envelope {
	t.Helper()
	var raw []byte
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT envelope FROM event_outbox
			WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2 ORDER BY seq DESC LIMIT 1`,
			eventType, f.deal.String()).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the %s event: %v", eventType, err)
	}
	var env events.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the %s event: %v", eventType, err)
	}
	return env
}

func (f *chatReleaseEnv) handle(t *testing.T, env events.Envelope) {
	t.Helper()
	if err := f.sync.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling %s: %v", env.Type, err)
	}
}

func (f *chatReleaseEnv) linkRetired(t *testing.T) bool {
	t.Helper()
	var retired bool
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT archived_at IS NOT NULL FROM chat_link WHERE id = $1`, f.link).Scan(&retired)
	}); err != nil {
		t.Fatal(err)
	}
	return retired
}

func TestARestoredDealLeavesItsFreedChannelFree(t *testing.T) {
	f := setupChatRelease(t)
	f.archive(t)
	f.handle(t, f.outboxed(t, "deal.archived"))
	f.restore(t)
	f.handle(t, f.outboxed(t, "deal.restored"))

	if !f.linkRetired(t) {
		t.Fatal("restoring the deal brought its channel back")
	}
}

func TestAnArchiveHandledAfterTheRestoreLeavesTheDealItsChannel(t *testing.T) {
	f := setupChatRelease(t)
	f.archive(t)
	archived := f.outboxed(t, "deal.archived")
	f.restore(t)
	f.handle(t, archived)

	if f.linkRetired(t) {
		t.Fatal("a late archive event retired the channel of a live deal")
	}
}

func TestADealARetentionPolicyArchivesFreesItsChannel(t *testing.T) {
	f := setupChatRelease(t)
	f.exec(t, `UPDATE deal SET status = 'lost', lost_reason = 'price',
		closed_at = now() - interval '30 days', created_at = now() - interval '30 days' WHERE id = $1`, f.deal)
	f.exec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action, enabled)
		VALUES ('deal', 'lost', 1, 'archive', true)
		ON CONFLICT (object_type, category) DO UPDATE SET retain_days = 1, action = 'archive', enabled = true`)
	integration.SettleIntoInstall(t, f.e)
	svc := NewRetentionServiceFor(InstallationDB(f.e.Pool), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(principal.SystemActing(f.e.Admin(), "system")); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	applied := f.outboxed(t, "retention.applied")
	stream, err := events.StreamFor(applied.Type)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Groups(), func(g events.Group) bool {
		return g.Name == "cg:chat-links" && slices.Contains(g.Streams, stream)
	}) {
		t.Fatalf("cg:chat-links does not read %s, where retention.applied rides", stream)
	}
	f.handle(t, applied)
	if !f.linkRetired(t) {
		t.Fatal("a deal a retention policy archived still holds its channel")
	}
}
