// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// ProviderChargedCredits is what a provider_run_reservation row, aliased r,
// counts as spent. The spend ledger and a contact's bought marks both read it.
//
// An unreconciled hold counts at its full reserved amount. The provider has not
// said what it charged, and assuming a refund would understate the bill while
// a customer decides whether to keep spending.
//
//nolint:gosec // G101 false positive: a SQL fragment naming credit columns, not a credential.
const ProviderChargedCredits = `COALESCE(r.actual_credits, r.reserved_credits)`

// ProviderRunCharges says a provider_run, aliased run, may count as a charge.
// A skipped or cancelled run bought nothing.
const ProviderRunCharges = `run.state <> 'skipped' AND run.state <> 'cancelled'`
