// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// promotionTouches lists, by type, the records an agent's promotion writes,
// links or relinks, including those its details name only as text.
func promotionTouches(lead ids.UUID, plan contacts.LeadContactChange) map[string][]ids.UUID {
	touched := map[string][]ids.UUID{entityLead: {lead}}
	add := func(entityType string, id *ids.UUID) {
		if id != nil && !id.IsZero() {
			touched[entityType] = append(touched[entityType], *id)
		}
	}
	add(entityContact, &plan.ContactID.UUID)
	if plan.ReviewContact != nil {
		add(entityContact, &plan.ReviewContact.UUID)
	}
	for _, company := range []*ids.CompanyID{plan.Employer.CompanyID, plan.EmployerPreview.CompanyID} {
		if company != nil {
			add(entityCompany, &company.UUID)
		}
	}
	activity := string(datasource.EntityActivity)
	if plan.Evidence != nil {
		add(activity, &plan.Evidence.UUID)
	}
	for _, moved := range plan.Activities {
		add(activity, &moved.ID)
	}
	for _, basis := range plan.ConsentLinks.CommunicationBases {
		add(activity, basis.SourceActivityID)
	}
	return touched
}
