// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A project's Documents holds its own uploads and the files of the activities
// filed under it, and nothing filed elsewhere. Every row comes from the real
// activity and upload writers.
func TestAProjectListsItsOwnUploadsAndTheFilesOfItsActivities(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	store, _ := attachmentStore(e)
	company := e.SeedCompany(t, "Acme", nil)
	project := seedProject(admin, t, e, "Rollout", company, nil).ID.UUID
	other := seedProject(admin, t, e, "Other rollout", company, nil).ID.UUID

	note := func(on ids.UUID) ids.UUID {
		subject := "Kickoff"
		row, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
			Kind: "note", Subject: &subject,
			Links: []activities.ActivityLinkInput{{EntityType: "project", EntityID: on}},
		})
		if err != nil {
			t.Fatalf("log a note on the project: %v", err)
		}
		return ids.UUID(row.Id)
	}
	upload := func(entityType string, on ids.UUID, name string) ids.UUID {
		att, err := store.UploadAttachment(admin, activities.AttachmentInput{
			EntityType: entityType, EntityID: on, Filename: name, Content: bytes.NewReader([]byte("%PDF-1.4 " + name)),
		})
		if err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
		return ids.UUID(att.Id)
	}
	own := upload("project", project, "plan.pdf")
	onNote := upload("activity", note(project), "minutes.pdf")
	elsewhere := upload("activity", note(other), "theirs.pdf")

	rows, _, err := store.ListAttachments(admin, "project", project, nil, nil)
	if err != nil {
		t.Fatalf("project documents: %v", err)
	}
	listed := map[ids.UUID]bool{}
	for _, row := range rows {
		listed[ids.UUID(row.Id)] = true
	}
	if !listed[own] {
		t.Error("the project does not list a file uploaded to it")
	}
	if !listed[onNote] {
		t.Error("the project does not list the file of a note filed under it")
	}
	if listed[elsewhere] {
		t.Error("the project lists the file of a note filed under another project")
	}
}
