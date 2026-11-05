// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *transcriptEnv) linkTranscriptContact(t *testing.T, contact ids.UUID) {
	t.Helper()
	original, err := e.Activities.GetActivity(e.Admin(), e.activity, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	if original.Links != nil {
		for _, link := range *original.Links {
			links = append(links, activities.ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
		}
	}
	meeting, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: original.Subject, Body: original.Body,
		SourceSystem: original.SourceSystem, Source: "manual", OccurredAt: &original.OccurredAt,
		Links: links, AttendeeUserIDs: []ids.UserID{ids.From[ids.UserKind](e.Rep2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), e.activity, nil); err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
}
