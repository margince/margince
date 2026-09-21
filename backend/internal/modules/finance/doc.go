// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package finance owns the ingested finance mirror (ADR-0083/A128,
// finance-ingestion.md): a bounded READ-ONLY import of an accounting
// source, so the company page can answer whether a customer actually
// pays us, and on time.
//
// It is a mirror, not an integration. The capability exposes no create
// or update action on a finance record at all, and that posture is
// expressed as the ABSENCE of a grant on the permission surface
// (FIN-DDL-N-1) rather than as a runtime check — there is no code path
// to reach and no flag to set wrongly. ADR-0094's removal of invoice
// creation is unchanged in every particular: reading our own already
// issued invoices back out of our own accounting system creates no
// artifact and asserts no tax position.
//
// The mirror is subordinate. A customer link maps two identifiers and
// never merges: an accounting customer never becomes a company,
// and an unmapped one is a visible state rather than an auto-created
// company.
//
// WHOEVER WRITES THE FIRST CUSTOMER LINK OWES A DECISION ABOUT MERGING.
// Nothing creates a finance_customer_link today — the sync reads them and
// the company merge moves them, but no code path makes one, so the case
// below is currently unreachable. It becomes reachable with the mapping
// writer, and it is silent when it arrives:
//
// finance_customer_link_company_ux admits one LIVE link per
// (connection, company). Merging two companies that each hold one on the
// same connection therefore cannot move both. The merge repoints the
// invoices and payments onto the survivor, moves the link only into a
// vacancy, and deliberately leaves the loser's link live rather than
// archiving it — readLinks (sync.go) reads unarchived links only, so
// archiving would stop syncing that external customer with nothing
// anywhere saying why. The consequence is that the next sync resolves
// that customer to the ARCHIVED company and moves its invoices back off
// the survivor. The merge looks correct and a later sync undoes it.
//
// The fix is not the merge's to make: it needs either a rule for which
// link wins, or a cardinality that admits both, which is a migration.
// Decide it with the writer, and cover it with a test that merges two
// linked companies and then syncs.
//
// Money is integer minor units in the issued currency, converted through
// the existing effective-dated rate sheet and frozen on issue date
// (DM-FX-4). A missing rate is the existing refusal — never a silent
// conversion and never a zero.
//
// Tables owned: finance_connection, finance_external_customer,
// finance_customer_link, finance_invoice, finance_payment.
package finance
