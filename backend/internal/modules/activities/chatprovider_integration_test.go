// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestChatProviderFilterUsesVisibleRecapsAndPreservesLegacySource(t *testing.T) {
	e := setupPromises(t)
	company, hidden := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO company(id,display_name,owner_id,visibility,source,captured_by) VALUES($1,'Visible account',$3,'workspace','manual','system'),($2,'Hidden account',$4,'owner','manual','system')`, company, hidden, e.rep, e.other)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	system := principal.SystemActing(e.as(), "system:chat_links")
	for _, provider := range []string{"dispact", "slack", "teams", "legacy", "hidden", "erased"} {
		parent := company
		raw := map[string]any{"provider": provider, "channel_name": "Berlin pilot", "url": "https://fixture.slack.com/archives/C_PILOT/p1700000000000001", "connection_id": ids.NewV7().String(), "message_count": 2, "ended_at": "2026-06-01T09:00:00Z"}
		if provider == "legacy" {
			delete(raw, "provider")
		}
		if provider == "hidden" {
			parent = hidden
			raw["provider"] = "slack"
		}
		subject, body, source, key := "Pilot approval", "Legal approval is still required.", provenance.ChatSummarySource, provider
		note, _, err := store.LogActivity(system, LogActivityInput{Kind: "note", Subject: &subject, Body: &body, Source: provenance.ChatSummaryOrigin, SourceSystem: &source, SourceID: &key, Raw: &raw, Links: []ActivityLinkInput{{EntityType: "company", EntityID: parent}}})
		if err != nil {
			t.Fatal(err)
		}
		if provider == "erased" {
			e.exec(t, `UPDATE activity SET raw=NULL,body=NULL WHERE id=$1`, note.Id)
		}
	}
	for provider, want := range map[string]int{"dispact": 2, "slack": 1, "teams": 1} {
		rows, page, err := store.ListActivities(e.as(), ListActivitiesInput{ChatProvider: &provider})
		if err != nil || len(rows) != want || page.HasMore {
			t.Fatalf("provider %s rows=%d page=%+v error=%v", provider, len(rows), page, err)
		}
		for _, row := range rows {
			if row.ChatEpisode == nil || row.ChatEpisode.Url != "" || row.ChatEpisode.ChannelName == nil || *row.ChatEpisode.ChannelName != "Berlin pilot" || row.SourceId != nil {
				t.Fatalf("recap context lost or private binding exposed: %+v", row)
			}
		}
		count, err := store.CountActivities(e.as(), ListActivitiesInput{ChatProvider: &provider})
		if err != nil || count != want {
			t.Fatalf("provider %s count=%d error=%v", provider, count, err)
		}
	}
	kind, provider := "company", "slack"
	rows, page, err := store.ListActivities(e.as(), ListActivitiesInput{EntityType: &kind, EntityID: &company, ChatProvider: &provider, Limit: new(1)})
	if err != nil || len(rows) != 1 || page.HasMore {
		t.Fatalf("hidden matches affected company paging: rows=%d page=%+v error=%v", len(rows), page, err)
	}
}
