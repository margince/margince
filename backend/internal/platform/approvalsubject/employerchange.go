// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// KindEmployerChange asks a contact's owner whether the contact moved to the
// company their newest mail points at.
const KindEmployerChange = "employer_change"

// EmployerChange is that question. OwnerID decides it. FromRelationshipID is
// the current primary employment when it was asked; accepting ends it and
// makes ToCompanyID current. Domain or Signature is the evidence a human
// judges, and ObservedOn is the day of the mail that carried it.
type EmployerChange struct {
	ContactID          ids.UUID `json:"contact_id"`
	OwnerID            ids.UUID `json:"owner_id"`
	ContactName        string   `json:"contact_name"`
	FromRelationshipID ids.UUID `json:"from_relationship_id"`
	FromCompanyID      ids.UUID `json:"from_company_id"`
	FromCompanyName    string   `json:"from_company_name"`
	ToCompanyID        ids.UUID `json:"to_company_id"`
	ToCompanyName      string   `json:"to_company_name"`
	ActivityID         ids.UUID `json:"activity_id"`
	ObservedOn         string   `json:"observed_on"`
	Domain             string   `json:"domain,omitempty"`
	Signature          string   `json:"signature,omitempty"`
}
