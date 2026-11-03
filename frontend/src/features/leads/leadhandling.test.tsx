/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { labelsIn } from "../../testing/shippedlabels.testkit";
import { useLeadDisqualifyReasons } from "./leadhandling";

afterEach(() => {
  vi.unstubAllGlobals();
});

function reason(id: string, label: string, labelKey: string | null) {
  return {
    id,
    label,
    label_key: labelKey,
    sort_order: 10,
    active: true,
    system: labelKey !== null,
    lead_count: 0,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

it("names the built-in disqualification reasons in the reader's language", async () => {
  const labels = await labelsIn(
    "vi",
    "/lead-disqualify-reasons",
    [
      reason("r1", "No budget", "leadReasons.builtin.no_budget"),
      reason("r2", "Went with an agency", null),
    ],
    useLeadDisqualifyReasons,
  );
  expect(labels).toEqual(["Không có ngân sách", "Went with an agency"]);
});
