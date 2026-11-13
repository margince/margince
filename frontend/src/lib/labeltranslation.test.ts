// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { translatedLabel, translatedPipeline } from "./labeltranslation";

type Pipeline = components["schemas"]["Pipeline"];

const PIPELINE: Pipeline = {
  id: "p-1",
  name: "Sales",
  is_default: true,
  position: 1,
  stages: [
    {
      id: "s-1",
      pipeline_id: "p-1",
      name: "Closed Won",
      position: 1,
      semantic: "won",
      win_probability: 100,
      name_translations: { vi: "Đã thắng" },
    },
    {
      id: "s-2",
      pipeline_id: "p-1",
      name: "Proposal Sent",
      position: 2,
      semantic: "open",
      win_probability: 50,
    },
  ],
};

describe("translatedPipeline", () => {
  it("names a stage in the reader's language when the admin gave one", () => {
    const vi = translatedPipeline(PIPELINE, "vi");
    expect(vi.stages?.map((stage) => stage.name)).toEqual([
      "Đã thắng",
      "Proposal Sent",
    ]);
  });

  it("keeps the stored name for a reader whose language has none", () => {
    const en = translatedPipeline(PIPELINE, "en");
    expect(en.stages?.map((stage) => stage.name)).toEqual([
      "Closed Won",
      "Proposal Sent",
    ]);
  });
});

describe("translatedLabel", () => {
  it("falls back to the stored label for a blank translation", () => {
    expect(translatedLabel("Services", { vi: "  " }, "vi")).toBe("Services");
  });
});
