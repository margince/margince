// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CustomFieldChoiceRemovers names, for every object a custom field attaches
// to, the module that owns its table and clears a removed choice off it.
func CustomFieldChoiceRemovers() map[string]customfields.ChoiceRemover {
	return map[string]customfields.ChoiceRemover{
		string(fieldcatalog.TargetContact):  contacts.RemoveContactChoices,
		string(fieldcatalog.TargetCompany):  contacts.RemoveCompanyChoices,
		string(fieldcatalog.TargetLead):     contacts.RemoveLeadChoices,
		string(fieldcatalog.TargetDeal):     deals.RemoveCustomChoices,
		string(fieldcatalog.TargetProject):  projects.RemoveCustomChoices,
		string(fieldcatalog.TargetContract): contracts.RemoveCustomChoices,
		string(fieldcatalog.TargetActivity): activities.RemoveCustomChoices,
	}
}
