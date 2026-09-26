// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { valueLabel } from "./analytics.questions.values";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

describe("a dimension value in the reader's words", () => {
  it("reads a status by whose status it is", () => {
    expect(valueLabel(t, "win-loss", "status", "won")).toBe("Won");
    expect(valueLabel(t, "leads-by-status", "status", "promoted")).toBe(
      "Qualified",
    );
  });

  it("reuses the words the timeline, forecast, meetings and projects use", () => {
    expect(valueLabel(t, "activities-by-kind", "kind", "meeting")).toBe(
      "Meeting",
    );
    expect(valueLabel(t, "forecast", "forecast_category", "best_case")).toBe(
      "Best case",
    );
    expect(
      valueLabel(t, "activities-by-kind", "meeting_status", "no_show"),
    ).toBe("No-show");
    expect(valueLabel(t, "projects-by-phase", "phase", "delivering")).toBe(
      translate("en", "project.phase.delivering"),
    );
    expect(valueLabel(t, "leads-by-status", "source", "webform")).toBe(
      "Web form",
    );
  });

  it("has no word for a value no screen names yet", () => {
    expect(valueLabel(t, "deals-by-stage", "status", "open")).toBeNull();
    expect(
      valueLabel(t, "activities-by-kind", "direction", "inbound"),
    ).toBeNull();
    expect(valueLabel(t, "win-loss", "size_band", "11-50")).toBeNull();
  });
});
