/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { AnalyticsScreen } from "./analytics";
import { render, reportsStub } from "./analytics.testkit";
import { stagesOfEveryPipeline } from "./pipelinestages";

type Stage = components["schemas"]["Stage"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const stage = (
  id: string,
  pipeline: string,
  name: string,
  position: number,
): Stage => ({
  id,
  pipeline_id: pipeline,
  name,
  position,
  semantic: "open",
  win_probability: 20,
});

const TWO_PIPELINES = {
  data: [
    {
      id: "other",
      name: "Partners",
      is_default: false,
      position: 1,
      stages: [stage("other-s1", "other", "Intake", 1)],
    },
    {
      id: "pl",
      name: "Sales",
      is_default: true,
      position: 0,
      stages: [stage("pl-s1", "pl", "Qualify", 1)],
    },
  ],
  page: { next_cursor: null },
};

describe("the pipeline stage table", () => {
  // The report counts deals in every pipeline, so a stage outside the default
  // pipeline must be named, not printed as its id.
  it("names a stage of a pipeline that is not the default", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        pipelines: TWO_PIPELINES,
        stageRows: [
          { stage_id: "pl-s1", raw_minor: 100, deal_count: 1 },
          { stage_id: "other-s1", raw_minor: 200, deal_count: 2 },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));
    expect((await screen.findAllByText("Intake")).length).toBeGreaterThan(0);
    expect(screen.queryByText("other-s1")).toBeNull();
  });
});

describe("stagesOfEveryPipeline", () => {
  it("tells apart a stage name two pipelines share", () => {
    const names = stagesOfEveryPipeline([
      {
        name: "Sales",
        is_default: true,
        stages: [stage("a", "pl", "Qualify", 1)],
      },
      { name: "Partners", stages: [stage("b", "o", "Qualify", 1)] },
    ]).map((entry) => entry.name);
    expect(names).toEqual(["Sales · Qualify", "Partners · Qualify"]);
  });

  it("lists the default pipeline first and keeps each ladder in order", () => {
    const stages = stagesOfEveryPipeline([
      {
        is_default: false,
        stages: [
          stage("o2", "other", "Won", 5000),
          stage("o1", "other", "Intake", 2),
        ],
      },
      { is_default: true, stages: [stage("p1", "pl", "Qualify", 1)] },
    ]);
    expect(stages.map((entry) => entry.name)).toEqual([
      "Qualify",
      "Intake",
      "Won",
    ]);
    expect(stages.map((entry) => entry.position)).toEqual([0, 1, 2]);
  });
});
