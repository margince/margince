// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { dealRings } from "./dealrings";

type DealStatusCard = components["schemas"]["DealStatusCard"];
type DealCoverage = components["schemas"]["DealCoverage"];

const NOW = Date.parse("2026-08-13T09:00:00Z");

const card: DealStatusCard = {
  deal_id: "d-1",
  story: { sentences: [] },
  stage_entered_at: "2026-06-18T09:00:00Z",
  stage_usual_days: 30.4,
  generated_at: "2026-08-13T08:00:00Z",
  generated_by: "model",
};

const coverage: DealCoverage = {
  deal_id: "d-1",
  stakeholders: [{ contact_id: "p-1", role: "champion", engaged: true }],
  our_side: [],
  risks: [
    { kind: "coverage_gap", summary: "No economic buyer" },
    { kind: "coverage_gap", summary: "No technical evaluator" },
    { kind: "going_cold", summary: "Quiet for 12 days" },
  ],
  sections_omitted: [],
};

describe("a deal's rings", () => {
  it("fill the stage ring to the usual time and count the roles named", () => {
    expect(dealRings(card, coverage, NOW)).toEqual([
      { key: "stage", value: 30, max: 30, tone: "warning", reading: 56 },
      { key: "committee", value: 1, max: 3, tone: "warning", reading: 1 },
    ]);
  });

  it("draw no stage ring below the history a usual time needs", () => {
    const young = { ...card, stage_usual_days: null };
    expect(dealRings(young, undefined, NOW)).toEqual([]);
  });

  it("read a deal inside its usual time as calm", () => {
    const fresh = { ...card, stage_entered_at: "2026-08-03T09:00:00Z" };
    expect(dealRings(fresh, undefined, NOW)[0]).toEqual({
      key: "stage",
      value: 10,
      max: 30,
      tone: "calm",
      reading: 10,
    });
  });
});
