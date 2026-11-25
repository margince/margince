// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// These tests pin what a unit's chat message persists through the extension ingress.
// A change to the shared filing code that moves its key, audience, audit, outbox or
// identity rows fails here by name.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func ingestChannelMessageFor(t *testing.T, e *ingressEnv, key, account string) extension.Result {
	t.Helper()
	result, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(e.member.String()),
		aChannelMessage(key, "someone@gmail.com", account))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	return result
}

func TestAUnitsChannelMessageIsFiledUnderItsExtensionSourceKeyAndWriteShape(t *testing.T) {
	e := setupIngress(t)
	registerProbeTransport(t, e)

	result := ingestChannelMessageFor(t, e, "ws-7:5001", "key-account-1")
	id := ids.MustParse(result.Ref.ID)

	for column, want := range map[string]string{
		"source_system":    ingressProbeSource,
		"source_id":        "ws-7:5001",
		"source":           ingressProbeSource,
		"kind":             "message",
		"channel_provider": ingressProbeProvider,
		"audience":         "workspace",
	} {
		var got string
		e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT coalesce(`+column+`::text, '') FROM activity WHERE id = $1`, id).Scan(&got)
		})
		if got != want {
			t.Errorf("activity.%s = %q, want %q", column, got, want)
		}
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, id); n != 1 {
		t.Errorf("%d audit rows for the new activity, want 1", n)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1`, id.String()); n != 1 {
		t.Errorf("%d outbox events for the new activity, want 1", n)
	}
	if n := e.countAsWorkspace(t, `
		SELECT count(*) FROM contact_channel_identity
		 WHERE provider = $1 AND channel_user_id = 'key-account-1' AND archived_at IS NULL AND contact_id IS NOT NULL`,
		ingressProbeProvider); n != 1 {
		t.Errorf("%d live channel identities for the unit's account, want 1", n)
	}
}

func TestAReplayedUnitChannelMessageLandsNothingNew(t *testing.T) {
	e := setupIngress(t)
	registerProbeTransport(t, e)
	first := ingestChannelMessageFor(t, e, "ws-7:5002", "key-account-2")
	audits := e.countAsWorkspace(t, `SELECT count(*) FROM audit_log`)
	events := e.countAsWorkspace(t, `SELECT count(*) FROM event_outbox`)
	contacts := e.countAsWorkspace(t, `SELECT count(*) FROM contact`)
	identities := e.countAsWorkspace(t, `SELECT count(*) FROM contact_channel_identity`)

	second := ingestChannelMessageFor(t, e, "ws-7:5002", "key-account-2")

	if second.Ref.ID != first.Ref.ID {
		t.Errorf("the replay named %s, want the original %s", second.Ref.ID, first.Ref.ID)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM activity WHERE source_system = $1`, ingressProbeSource); n != 1 {
		t.Fatalf("%d activities after a replay, want 1", n)
	}
	for table, before := range map[string]int{
		"audit_log": audits, "event_outbox": events, "contact": contacts, "contact_channel_identity": identities,
	} {
		if after := e.countAsWorkspace(t, `SELECT count(*) FROM `+table); after != before {
			t.Errorf("%s holds %d rows after a replay, want the %d it held before", table, after, before)
		}
	}
}
