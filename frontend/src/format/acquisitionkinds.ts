// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a contact was obtained: the closed vocabulary of
// contact_acquisition_evidence.kind, mirrored from the contacts module.
// backend/gates/frontendacquisitionkinds_test.go fails when the two differ.
export const ACQUISITION_KINDS = [
  "subject_initiated",
  "customer_contract",
  "requested_quote_or_meeting",
  "in_person_permission",
  "referral",
  "event_or_form",
  "public_or_business_source",
  "purchased_or_imported",
  "unknown_legacy",
  "crm_migration",
  "mailbox_history",
] as const;

export type AcquisitionKind = (typeof ACQUISITION_KINDS)[number];

const KNOWN: ReadonlySet<string> = new Set(ACQUISITION_KINDS);

// The wire carries a plain string, and a newer server may send a kind this
// build has no words for.
export function isAcquisitionKind(kind: string): kind is AcquisitionKind {
  return KNOWN.has(kind);
}
