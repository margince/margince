// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// OwnCompanyDomainsSQL selects the domains the installation's own company
// claims. It reads another module's tables, which reads may do: the own
// company is the only place a human says a domain is ours.
const OwnCompanyDomainsSQL = `
	SELECT d.domain
	  FROM company_domain d
	  JOIN company o ON o.id = d.company_id
	 WHERE o.is_anchor AND o.archived_at IS NULL AND d.archived_at IS NULL`

// TrustedOwnDomainsSQL selects the domains a human vouched for: those the own
// company claims and those an administrator confirmed. Capture decides which
// mail it stores with it, and the sender read arm asks it at read time.
const TrustedOwnDomainsSQL = `SELECT domain FROM workspace_email_domain WHERE verified UNION ` + OwnCompanyDomainsSQL
