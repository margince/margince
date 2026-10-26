import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { stageRank } from "./companyoverviewrows";

type Pipeline = components["schemas"]["Pipeline"];
type Stage = NonNullable<Pipeline["stages"]>[number];

function stage(
  id: string,
  position: number,
  semantic: Stage["semantic"],
): Stage {
  return {
    id,
    pipeline_id: "p-1",
    name: id,
    position,
    semantic,
    win_probability: 0,
  };
}

const pipeline: Pipeline = {
  id: "p-1",
  name: "Sales",
  is_default: true,
  position: 0,
  stages: [
    stage("won", 6, "won"),
    stage("proposal", 3, "open"),
    stage("discovery", 1, "open"),
    stage("negotiation", 4, "open"),
    stage("lost", 7, "lost"),
  ],
};

describe("stageRank", () => {
  it("counts a deal's stage among its pipeline's open stages, by position", () => {
    expect(stageRank("proposal", [pipeline])).toEqual({ rank: 2, of: 3 });
  });

  it("claims no rank for a stage it cannot find, or before the read lands", () => {
    expect(stageRank("elsewhere", [pipeline])).toBeUndefined();
    expect(stageRank("proposal", undefined)).toBeUndefined();
    expect(stageRank(null, [pipeline])).toBeUndefined();
  });
});
