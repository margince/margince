// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestRetainedChatRecapStillFollowsLocalRetentionAfterUnlink(t *testing.T) {
	f := setupChatRelease(t)
	acts := activities.NewStore(f.e.DB())
	subject, body := "Pilot remains conditional", "The pilot requires Legal approval; that approval has not been granted."
	source, key := provenance.ChatSummarySource, "chosen:retention-fixture"
	occurred := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	metadata := map[string]any{"provider": "dispact", "link_id": f.link.String(), "channel_id": "acme", "root_id": "fixture-root", "message_count": 2, "ended_at": occurred.Format(time.RFC3339)}
	note, created, err := acts.LogActivity(f.e.Admin(), activities.LogActivityInput{Kind: "note", Subject: &subject, Body: &body, Source: "chat_links", SourceSystem: &source, SourceID: &key, Raw: &metadata, OccurredAt: &occurred, Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: f.deal}}})
	if err != nil || !created {
		t.Fatalf("write recap: created=%v error=%v", created, err)
	}
	links := chatlinks.NewStore(f.e.DB(), nil, unqueuedChatJobs{}, chatsync.Viewers)
	if err := links.Unlink(f.e.Admin(), "deal", f.deal); err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(note.Id))
	retained, err := acts.GetActivity(f.e.Admin(), id, storekit.LiveOnly)
	if err != nil || retained.Body == nil || *retained.Body != body {
		t.Fatalf("unlink lost eligible history: %v", err)
	}
	f.exec(t, `INSERT INTO retention_policy(object_type,category,retain_days,action,enabled) VALUES('activity',NULL,1,'erase',true)`)
	integration.SettleIntoInstall(t, f.e)
	svc := NewRetentionServiceFor(f.e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(principal.SystemActing(f.e.Admin(), "system")); err != nil {
		t.Fatal(err)
	}
	erased, err := acts.GetActivity(f.e.Admin(), id, storekit.IncludeArchived)
	if err != nil || erased.Body != nil || erased.Raw != nil || erased.Subject != nil && *erased.Subject == subject {
		t.Fatalf("retention left recap content after unlink: %+v %v", erased, err)
	}
}
