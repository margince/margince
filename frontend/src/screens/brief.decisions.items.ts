// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { DecisionDeckItem } from "../design-system/decisiondeck";

type Approval = components["schemas"]["Approval"];

export function deckItems(approvals: readonly Approval[]): DecisionDeckItem[] {
  const seen = new Set<string>();
  const out: DecisionDeckItem[] = [];
  for (const approval of approvals) {
    const bundleId = approval.bundle_id;
    if (!bundleId) {
      out.push({ kind: "single", id: approval.id, approval });
      continue;
    }
    if (seen.has(bundleId)) {
      continue;
    }
    seen.add(bundleId);
    const members = approvals.filter((other) => other.bundle_id === bundleId);
    out.push(
      members.length === 1
        ? { kind: "single", id: members[0].id, approval: members[0] }
        : { kind: "bundle", id: bundleId, bundleId, members },
    );
  }
  return out;
}
