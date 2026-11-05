// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";

export const domainReasonLabels: Record<string, MessageKey> = {
  unevidenced: "blockedDomains.source.unevidenced",
  stale_evidence: "blockedDomains.source.staleEvidence",
  near_duplicate: "blockedDomains.source.nearDuplicate",
  forwarded_identity: "blockedDomains.source.forwardedIdentity",
  vendor_verdict: "blockedDomains.source.vendorVerdict",
  reserved_domain: "blockedDomains.source.reservedDomain",
};
