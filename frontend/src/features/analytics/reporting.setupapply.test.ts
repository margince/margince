// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  goalWithProposal,
  ruleWithProposal,
  selectionWithProposal,
} from "./reporting.setupapply";
import type { SetupProposal } from "./reporting.setupproposal";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";

const proposal: SetupProposal = {
  site: "initiative_setup",
  generated_by: "model",
  name: "Linden push",
  selection: {
    product_id: "11111111-1111-4111-8111-111111111111",
    country: "DE",
  },
  objective: {
    metric: "bookings_won",
    target_value: 250000,
    deadline: "2026-12-31",
  },
  sources: [],
  clarifications: [],
  unsupported: [],
};

describe("applying a proposal", () => {
  it("replaces what the words named, keeps a typed name and leaves the rest", () => {
    const goal = goalWithProposal(
      {
        name: "My push",
        description: "Push Linden",
        objective: "open_pipeline",
        target: "5",
        deadline: "",
        includeFuture: false,
      },
      proposal,
    );
    expect(goal).toEqual({
      name: "My push",
      description: "Push Linden",
      objective: "bookings_won",
      target: "250000",
      deadline: "2026-12-31",
      includeFuture: false,
    });
    expect(
      ruleWithProposal(
        { product: "", industry: "software", country: "AT" },
        proposal,
      ),
    ).toEqual({
      product: "11111111-1111-4111-8111-111111111111",
      industry: "software",
      country: "DE",
    });
  });

  it("takes a blank name from the proposal and refuses an objective the drawer does not offer", () => {
    const goal = goalWithProposal(
      {
        name: "",
        description: "",
        objective: "open_pipeline",
        target: "",
        deadline: "",
        includeFuture: true,
      },
      { ...proposal, objective: { metric: "closed_win_rate" } },
    );
    expect(goal.name).toBe("Linden push");
    expect(goal.objective).toBe("open_pipeline");
    expect(goal.target).toBe("");
  });

  it("keeps the selection's own metrics and charts when the words named none", () => {
    const selection = reportingStoryEvaluation.selection;
    const next = selectionWithProposal(selection, proposal);
    expect(next.metrics).toBe(selection.metrics);
    expect(next.blocks).toBe(selection.blocks);
    expect(next.country).toBe("DE");
    expect(next.period).toBe(selection.period);
    const charted = selectionWithProposal(selection, {
      ...proposal,
      selection: { metrics: ["bookings_won"], blocks: ["bookings_trend"] },
    });
    expect(charted.metrics).toEqual(["bookings_won"]);
    expect(charted.blocks).toEqual(["bookings_trend"]);
  });
});
