// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The one rule for who an email about a row goes to, case by case.
func TestTheEmailTargetRule(t *testing.T) {
	t.Parallel()
	contact := emailCandidate{id: ids.NewV7(), name: "Alice", address: "alice@buyer.example"}
	lead := emailCandidate{id: ids.NewV7(), name: "Lena", address: "lena@prospect.example"}
	otherLead := emailCandidate{id: ids.NewV7(), name: "Lars", address: "lars@prospect.example"}
	closedLead := emailCandidate{id: ids.NewV7(), name: "Old", address: "old@prospect.example", closed: true}
	company := emailCandidate{id: ids.NewV7()}
	const (
		task = crmcontracts.ActivityKindTask
		call = crmcontracts.ActivityKindCall
	)
	type want struct {
		named   bool
		target  ids.UUID
		address string
		refusal crmcontracts.ActivityEmailTargetRefusal
	}
	for _, tc := range []struct {
		name  string
		kind  crmcontracts.ActivityKind
		found emailCandidates
		want  want
	}{
		{
			"one contact", task,
			emailCandidates{contacts: []emailCandidate{contact}, leads: []emailCandidate{lead}},
			want{true, contact.id, contact.address, ""},
		},
		{
			"an archived contact", task,
			emailCandidates{contacts: []emailCandidate{{id: contact.id, closed: true}}},
			want{true, contact.id, "", crmcontracts.ActivityEmailTargetRefusalArchived},
		},
		{
			"a contact with no address", call,
			emailCandidates{contacts: []emailCandidate{{id: contact.id}}},
			want{true, contact.id, "", crmcontracts.ActivityEmailTargetRefusalNoAddress},
		},
		{
			"one live lead", task,
			emailCandidates{leads: []emailCandidate{lead}},
			want{true, lead.id, lead.address, ""},
		},
		{
			"a live and a closed lead, and a company", task,
			emailCandidates{leads: []emailCandidate{closedLead, lead}, companies: []emailCandidate{company}},
			want{true, lead.id, lead.address, ""},
		},
		{
			"two live leads", task,
			emailCandidates{leads: []emailCandidate{lead, otherLead}, companies: []emailCandidate{company}},
			want{},
		},
		{
			"a live lead with no address", task,
			emailCandidates{leads: []emailCandidate{{id: lead.id}}},
			want{true, lead.id, "", crmcontracts.ActivityEmailTargetRefusalNoAddress},
		},
		{
			"a closed lead and a company", task,
			emailCandidates{leads: []emailCandidate{closedLead}, companies: []emailCandidate{company}},
			want{true, company.id, "", ""},
		},
		{
			"a closed lead and no company", task,
			emailCandidates{leads: []emailCandidate{closedLead}},
			want{true, closedLead.id, "", crmcontracts.ActivityEmailTargetRefusalLeadClosed},
		},
		{
			"one company", task,
			emailCandidates{companies: []emailCandidate{company}},
			want{true, company.id, "", ""},
		},
		{
			"two companies", task,
			emailCandidates{companies: []emailCandidate{company, {id: ids.NewV7()}}},
			want{},
		},
		{"a call on a lead", call, emailCandidates{leads: []emailCandidate{lead}}, want{}},
		{"nothing", task, emailCandidates{}, want{}},
	} {
		target, named := pickEmailTarget(tc.kind, tc.found)
		got := want{named, target.EntityID, target.Address, target.Refusal}
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
