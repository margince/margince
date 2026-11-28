// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An archived profile takes its builds, versions and deltas with it. A queued
// build then never runs for it, and no history read lists its past.
func TestArchivingAVoiceProfileRetiresItsHistory(t *testing.T) {
	e := setupSendOutcomeStore(t)
	fx := e.seedDraft(t, draftOptions{})
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO voice_profile_version (voice_profile_id, profile_version, status, reason, source,
		    captured_by, builder_version, activation_policy_version, model_provider, model_name,
		    profile_json, evaluation_json, stats_json, voice_profile_md, source_count, source_hash)
		VALUES ($1, 1, 'active', 'manual', 'manual', 'system', 'v1', 'v1', 'test', 'test',
		    '{}', '{}', '{}', '', 0, 'h')`, fx.profile); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO voice_profile_delta (voice_profile_id, to_version, classification, activation_outcome, delta_json)
		VALUES ($1, 1, 'routine', 'auto_activated', '{}')`, fx.profile); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO voice_build (voice_profile_id, reason, status, source_count, source_hash)
		VALUES ($1, 'manual', 'queued', 0, 'h')`, fx.profile); err != nil {
		t.Fatal(err)
	}

	archiver := fx.actor
	archiver.Permissions.Objects = map[string]principal.ObjectGrant{"voice_profile": {Read: true, Delete: true}}
	if _, err := e.storeIn(fx.workspace).ArchiveProfile(principal.WithActor(fx.ctx, archiver), fx.profile, nil); err != nil {
		t.Fatalf("archive the profile: %v", err)
	}

	for _, table := range []string{"voice_build", "voice_profile_version", "voice_profile_delta"} {
		var live int
		if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM `+table+
			` WHERE voice_profile_id = $1 AND archived_at IS NULL`, fx.profile).Scan(&live); err != nil {
			t.Fatal(err)
		}
		if live != 0 {
			t.Errorf("%d %s rows stay live after their profile was archived", live, table)
		}
	}
}
