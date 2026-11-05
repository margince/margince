// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// addressDomainFoldDataTask writes the folded domain on contact and lead
// addresses stored before the company and duplicate matchers compared it.
// Until a row has it, those matchers read the address as having no domain.
func addressDomainFoldDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793135707_address_domains_folded", Version: 1,
		Title: "Fold the domains of contact and lead addresses", Requires: []string{"core/1793135706"},
		Steps: []datatask.Step{
			foldStep("fold_lead_domains", "contacts writes the same folded domain on the lead it inserts, without a separate audit",
				contacts.FoldLeadEmailDomainsTx, contacts.CountUnfoldedLeadEmailDomains),
			foldStep("fold_contact_email_domains", "contacts writes the same folded domain on the address it inserts, without a separate audit",
				contacts.FoldContactEmailDomainsTx, contacts.CountUnfoldedContactEmailDomains),
		},
	}
}
