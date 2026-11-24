// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isTemplateDraft, modelWrote } from "./composedraftcall";

describe("which writer drafted the words", () => {
  it("is a model only when the answer says a model wrote it", () => {
    expect(modelWrote({ ai_generated: true })).toBe(true);
    expect(modelWrote({ ai_generated: false })).toBe(false);
    expect(modelWrote(null)).toBe(false);
  });

  it("is a template when the server says its floor wrote the words", () => {
    expect(
      isTemplateDraft({ ai_generated: false, generated_by: "deterministic" }),
    ).toBe(true);
  });

  it("is no template when a model wrote it or the route cannot say", () => {
    expect(isTemplateDraft({ ai_generated: true, generated_by: "model" })).toBe(
      false,
    );
    // A reply draft carries no generated_by: no notice can name its writer.
    expect(isTemplateDraft({ ai_generated: false })).toBe(false);
    expect(isTemplateDraft(null)).toBe(false);
  });
});
