// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// mergeScopeTouches lists the records a merge's details quote by text alone.
// Those are moved signals, files and relationships, and the records its
// notices, commitments and handoffs are about.
func mergeScopeTouches(scope contacts.MergeSurvivorship) map[string][]ids.UUID {
	touched := map[string][]ids.UUID{}
	add := func(entityType string, id ids.UUID) {
		if !id.IsZero() {
			touched[entityType] = append(touched[entityType], id)
		}
	}
	for _, signal := range scope.Signals {
		add(tableSignal, ids.UUID(signal.Id))
	}
	for _, file := range scope.Files {
		add(previewEntityAttachment, file.ID)
	}
	for _, item := range scope.Relationships {
		add(string(datasource.EntityRelationship), ids.UUID(item.Relationship.Id))
	}
	for _, notice := range scope.Notices {
		if notice.Target != nil {
			add(notice.Target.Type, ids.UUID(notice.Target.Id))
		}
	}
	for _, commitment := range scope.Commitments {
		if commitment.LinkedRecord != nil {
			add(string(commitment.LinkedRecord.Type), ids.UUID(commitment.LinkedRecord.Id))
		}
	}
	for _, handoff := range scope.Handoffs {
		for entityType, id := range map[string]*ids.UUID{entityLead: handoff.LeadID, entityContact: handoff.ContactID, entityCompany: handoff.CompanyID, string(datasource.EntityDeal): handoff.DealID} {
			if id != nil {
				add(entityType, *id)
			}
		}
	}
	return touched
}
