// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Feature = components["schemas"]["AiFeatureRoute"];

// Decision-first activities lead, then those that declare a decision form and
// say why it is not used; the server's order holds inside each group.
export function decisionFirstOrder(rows: Feature[]): Feature[] {
  const rank = (row: Feature) =>
    row.decision_first ? 0 : row.decision_skip_reason ? 1 : 2;
  return rows
    .map((row, index) => ({ row, index }))
    .sort((a, b) => rank(a.row) - rank(b.row) || a.index - b.index)
    .map(({ row }) => row);
}
