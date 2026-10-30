import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import {
  assignableStages,
  type PartnerStage,
  partnerKindsLabel,
  partnerStageLabel,
} from "./partnerstages.queries";

const t = (key: Parameters<typeof translate>[1]) => translate("de", key);

function stage(key: string, active: boolean): PartnerStage {
  return {
    id: key,
    key,
    label: key,
    meaning: "candidate",
    sort_order: 0,
    active,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
  };
}

describe("the partner ladder a partner may move on", () => {
  it("offers the active stages and the retired one the partner stands on", () => {
    const stages = [stage("a", true), stage("b", false), stage("c", false)];
    expect(assignableStages(stages, "b").map((s) => s.key)).toEqual(["a", "b"]);
    expect(assignableStages(stages, undefined).map((s) => s.key)).toEqual([
      "a",
    ]);
  });

  it("reads a built-in stage in the reader's language and a renamed one as written", () => {
    expect(
      partnerStageLabel(
        {
          relationship_stage: "ready",
          stage_label: "Ready",
          stage_label_key: "partnerStages.builtin.ready",
        },
        t,
      ),
    ).toBe("Bereit");
    expect(
      partnerStageLabel(
        {
          relationship_stage: "ready",
          stage_label: "Startklar",
          stage_label_key: null,
        },
        t,
      ),
    ).toBe("Startklar");
  });

  it("names the kinds in one fixed order whatever order they came in", () => {
    expect(partnerKindsLabel(["referral", "hosting"], t)).toBe(
      "Hosting, Empfehlung",
    );
  });
});
