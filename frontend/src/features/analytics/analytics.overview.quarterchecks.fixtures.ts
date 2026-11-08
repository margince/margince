// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The quarter's open forecast checks for the populated Sales review. The seat
// only reads, so each check opens rather than answers. Names are invented.

import type { components } from "../../api/schema";
import type { CheckList } from "./analytics.overview.scenariodata";

type InputCheck = components["schemas"]["InputCheck"];

/**
 * Two deals that need someone this quarter, as listInputChecks answers them:
 * one past its close date, one with no next step.
 */
export function quarterChecks(clock: string): CheckList {
  const finding = (
    index: number,
    type: string,
    label: string,
    amount: number,
    severity: InputCheck["severity"],
  ): InputCheck => {
    const subjectID = `42000000-0000-4000-8000-00000000000${index}`;
    return {
      id: `43000000-0000-4000-8000-00000000000${index}`,
      type,
      subject_kind: "deal",
      subject_id: subjectID,
      subject: { type: "deal", id: subjectID, label },
      severity,
      affected_minor: amount,
      currency: "EUR",
      status: "open",
      first_seen_at: "2026-10-19T02:00:00Z",
      last_seen_at: clock,
      inputs_changed: false,
      destination: { screen: "forecast_review", mode: "read" },
    };
  };
  const destination = { screen: "forecast_review", mode: "read" } as const;
  return {
    data: [
      finding(
        1,
        "close_past",
        "Nordlicht Logistics · Fleet rollout",
        14_000_000,
        "high",
      ),
      finding(
        2,
        "no_next_step",
        "Vireo Health · Pilot extension",
        6_000_000,
        "medium",
      ),
    ],
    groups: [
      {
        type: "close_past",
        findings: 1,
        deals: 1,
        other_findings: 0,
        exposure: [{ currency: "EUR", amount_minor: 14_000_000 }],
        destination,
      },
      {
        type: "no_next_step",
        findings: 1,
        deals: 1,
        other_findings: 0,
        exposure: [{ currency: "EUR", amount_minor: 6_000_000 }],
        destination,
      },
    ],
    totals: { findings: 2, deals: 2, other_findings: 0 },
  };
}
