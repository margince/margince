// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// ownCompanyDomainsFrom is where the installation's own company claims its
// domains. It reads another module's tables, which reads may do: the own
// company is the only place a human says a domain is ours.
const ownCompanyDomainsFrom = `
	  FROM company_domain d
	  JOIN company o ON o.id = d.company_id
	 WHERE o.is_anchor AND o.archived_at IS NULL AND d.archived_at IS NULL`

// verifiedWorkspaceDomainsSQL is the domains an administrator confirmed. Both
// writers of workspace_email_domain store the domain folded.
const verifiedWorkspaceDomainsSQL = `SELECT domain FROM workspace_email_domain WHERE verified`

// OwnCompanyDomainsSQL selects the domains the own company claims, as typed.
const OwnCompanyDomainsSQL = `SELECT d.domain` + ownCompanyDomainsFrom

// TrustedOwnDomainsSQL selects the domains a human vouched for: those the own
// company claims and those an administrator confirmed. Capture decides which
// mail it stores with it, after owndomain.New folds them.
const TrustedOwnDomainsSQL = verifiedWorkspaceDomainsSQL + ` UNION ` + OwnCompanyDomainsSQL

// TrustedOwnDomainsFoldedSQL is the same set in folded form, for a condition
// that compares in SQL through owndomain.CoversSQL. A company domain written
// before its folded form existed is missing until the fold task has run.
const TrustedOwnDomainsFoldedSQL = verifiedWorkspaceDomainsSQL +
	` UNION SELECT d.domain_folded` + ownCompanyDomainsFrom + ` AND d.domain_folded IS NOT NULL`
