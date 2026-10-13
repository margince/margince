// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An assistant asks get_tag what retiring a word costs. A lead carrying the
// word is part of that cost, so the tool reports it as the admin screen does.
func TestGetTagCountsTheLeadsCarryingTheWord(t *testing.T) {
	e := integration.Setup(t)
	tag := seedTag(t, e, "Trade Fair", false)
	name, email := "Lena Lead", "lena@prospect.test"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
	seam := tagSeam(e.Pool)
	if err := seam.ApplyTag(e.Admin(), tag, "lead", ids.UUID(lead.Id)); err != nil {
		t.Fatalf("tagging the lead: %v", err)
	}

	detail, err := seam.GetTag(e.Admin(), tag)
	if err != nil {
		t.Fatalf("reading the tag: %v", err)
	}
	if detail.Leads != 1 {
		t.Errorf("get_tag counted %d leads carrying the word, want 1", detail.Leads)
	}
}
