/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
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

  it("names a stage of an archived pipeline and adds no row for its empty stages", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        archivedPipelines: [
          {
            id: "old",
            name: "Old partners",
            is_default: false,
            position: 2,
            archived_at: "2026-05-01T00:00:00Z",
            stages: [
              stage("old-s1", "old", "Legacy intake", 1),
              stage("old-s2", "old", "Legacy close", 2),
            ],
          },
        ],
        stageRows: [
          { stage_id: "pl-s1", raw_minor: 100, deal_count: 1 },
          { stage_id: "old-s1", raw_minor: 200, deal_count: 2 },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));
    const table = within(
      await screen.findByRole("table", { name: "Open deals by stage" }),
    );
    expect(
      await table.findByText("Old partners (retired) · Legacy intake"),
    ).toBeTruthy();
    expect(table.queryByText("old-s1")).toBeNull();
    expect(table.queryByText(/Legacy close/)).toBeNull();
  });
});

describe("stagesOfEveryPipeline", () => {
  const retired = (name: string) => `${name} (retired)`;
  const ARCHIVED_AT = "2026-05-01T00:00:00Z";

  it("tells apart a stage name two pipelines share", () => {
    const names = stagesOfEveryPipeline(
      [
        {
          name: "Sales",
          is_default: true,
          stages: [stage("a", "pl", "Qualify", 1)],
        },
        { name: "Partners", stages: [stage("b", "o", "Qualify", 1)] },
      ],
      retired,
    ).map((entry) => entry.name);
    expect(names).toEqual(["Sales · Qualify", "Partners · Qualify"]);
  });

  it("lists the default pipeline first and keeps each ladder in order", () => {
    const stages = stagesOfEveryPipeline(
      [
        {
          is_default: false,
          stages: [
            stage("o2", "other", "Won", 5000),
            stage("o1", "other", "Intake", 2),
          ],
        },
        { is_default: true, stages: [stage("p1", "pl", "Qualify", 1)] },
      ],
      retired,
    );
    expect(stages.map((entry) => entry.name)).toEqual([
      "Qualify",
      "Intake",
      "Won",
    ]);
    expect(stages.map((entry) => entry.position)).toEqual([0, 1, 2]);
  });

  it("lists an archived pipeline's stages last, marked retired, renaming no live stage", () => {
    const names = stagesOfEveryPipeline(
      [
        {
          name: "Old partners",
          archived_at: ARCHIVED_AT,
          stages: [stage("r1", "old", "Qualify", 1)],
        },
        { name: "Partners", stages: [stage("p1", "o", "Intake", 1)] },
        {
          name: "Sales",
          is_default: true,
          stages: [stage("s1", "pl", "Qualify", 1)],
        },
      ],
      retired,
    ).map((entry) => entry.name);
    expect(names).toEqual([
      "Qualify",
      "Intake",
      "Old partners (retired) · Qualify",
    ]);
  });

  it("tells an archived pipeline from a live one that took its name", () => {
    const names = stagesOfEveryPipeline(
      [
        {
          name: "Sales",
          is_default: true,
          stages: [stage("l1", "pl", "Qualify", 1)],
        },
        { name: "Partners", stages: [stage("p1", "o", "Qualify", 1)] },
        {
          name: "Sales",
          archived_at: ARCHIVED_AT,
          stages: [stage("a1", "old", "Qualify", 1)],
        },
      ],
      retired,
    ).map((entry) => entry.name);
    expect(names).toEqual([
      "Sales · Qualify",
      "Partners · Qualify",
      "Sales (retired) · Qualify",
    ]);
  });

  it("gives every stage its own label when two archived pipelines share a name", () => {
    const stages = stagesOfEveryPipeline(
      [
        {
          name: "Sales",
          archived_at: ARCHIVED_AT,
          stages: [stage("a1", "x", "Qualify", 1)],
        },
        {
          name: "Sales",
          archived_at: ARCHIVED_AT,
          stages: [stage("a2", "y", "Qualify", 1)],
        },
        {
          name: "Sales",
          is_default: true,
          stages: [stage("l1", "pl", "Sales (retired) · Qualify (2)", 1)],
        },
      ],
      retired,
    );
    const names = stages.map((entry) => entry.name);
    expect(new Set(names).size).toBe(stages.length);
    expect(names).toEqual([
      "Sales (retired) · Qualify (2)",
      "Sales (retired) · Qualify",
      "Sales (retired) · Qualify (3)",
    ]);
  });
});
