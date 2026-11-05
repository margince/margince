// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A report keeps the purpose its author typed, trimmed. An update that leaves
// it out keeps it, a blank one clears it, and one past 2000 characters is
// refused by name.
func TestASavedReportKeepsItsPurpose(t *testing.T) {
	f := reportingBusiness(t)
	purpose := "  Weekly pipeline review for the board.  "
	input := crmcontracts.ReportingReportInput{
		Name: "Board review", Description: &purpose, Audience: "private", Selection: f.selection(),
	}
	created, err := f.service.CreateReport(f.human, input)
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.service.GetReport(f.human, ids.UUID(created.Id))
	if err != nil {
		t.Fatal(err)
	}
	if read.Description == nil || *read.Description != "Weekly pipeline review for the board." {
		t.Fatalf("the report reads back description %v, want the trimmed purpose", read.Description)
	}

	input.Description = nil
	input.Name = "Board review, renamed"
	renamed, err := f.service.UpdateReport(f.human, ids.UUID(created.Id), created.Version, input)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Description == nil || *renamed.Description != "Weekly pipeline review for the board." {
		t.Fatalf("an update that left the description out changed it to %v, want it kept", renamed.Description)
	}

	blank := "   "
	input.Description = &blank
	updated, err := f.service.UpdateReport(f.human, ids.UUID(created.Id), renamed.Version, input)
	if err != nil {
		t.Fatal(err)
	}
	read, err = f.service.GetReport(f.human, ids.UUID(updated.Id))
	if err != nil {
		t.Fatal(err)
	}
	if read.Description != nil {
		t.Errorf("a blank description reads back as %q, want none", *read.Description)
	}

	long := strings.Repeat("p", 2001)
	input.Description = &long
	_, err = f.service.UpdateReport(f.human, ids.UUID(created.Id), updated.Version, input)
	requireRefusedField(t, err, "description")
}
