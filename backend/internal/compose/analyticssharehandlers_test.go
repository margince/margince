// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func listedShareFields(t *testing.T, share Share) map[string]any {
	t.Helper()
	body, err := json.Marshal(shareToWire(share))
	if err != nil {
		t.Fatalf("rendering the listed share: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("reading the listed share back: %v", err)
	}
	return fields
}

func TestAListedWorkspaceLiveShareNamesNoSubjectNoSnapshotAndNoToken(t *testing.T) {
	fields := listedShareFields(t, Share{
		ID: ids.NewV7(), Kind: shareKindLive, Target: "forecast",
		Scope:     forecasting.Scope{Kind: forecasting.ScopeWorkspace},
		ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
	})
	for _, absent := range []string{"scope_id", "snapshot_id", "token"} {
		if value, present := fields[absent]; present {
			t.Errorf("a listed workspace live share carries %s = %v; it has none to name", absent, value)
		}
	}
	if fields["scope_kind"] != forecasting.ScopeWorkspace {
		t.Errorf("scope_kind = %v, want %q", fields["scope_kind"], forecasting.ScopeWorkspace)
	}
}

func TestAListedTeamSnapshotShareNamesItsTeamAndItsSnapshot(t *testing.T) {
	team, snapshot := ids.NewV7(), ids.NewV7()
	fields := listedShareFields(t, Share{
		ID: ids.NewV7(), Kind: shareKindSnapshot, Target: "forecast",
		Scope:      forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &team},
		SnapshotID: &snapshot,
		ExpiresAt:  time.Now().Add(time.Hour), CreatedAt: time.Now(),
	})
	if fields["scope_id"] != team.String() {
		t.Errorf("scope_id = %v, want the team %s", fields["scope_id"], team)
	}
	if fields["snapshot_id"] != snapshot.String() {
		t.Errorf("snapshot_id = %v, want the snapshot %s", fields["snapshot_id"], snapshot)
	}
}
