// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

func (a *assembly) readLeads() error {
	data, page, err := a.svc.contacts.ListLeads(a.ctx, contacts.ListLeadsInput{
		CompanyID: &a.companyID, Limit: new(5),
	})
	if err != nil {
		return err
	}
	a.out.Leads = &crmcontracts.LeadListResponse{Data: data, Page: pageInfo(page)}
	return nil
}
