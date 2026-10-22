// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  badgeTone,
  bandTone,
  READING_TONES,
  ratingTone,
  readingColours,
  statTone,
} from "./readingtone";

describe("a reading's tone names its colours once", () => {
  it.each(["success", "warning", "danger"] as const)(
    "draws %s from its own state tokens",
    (tone) => {
      expect(readingColours(tone)).toEqual({
        mark: `var(--${tone})`,
        ink: `var(--${tone}Text)`,
        wash: `var(--${tone}Bg)`,
      });
    },
  );

  it("draws an unjudged reading in no state colour and no wash", () => {
    expect(readingColours("neutral")).toEqual({
      mark: "var(--borderStrong)",
      ink: "var(--textSecondary)",
      wash: "transparent",
    });
  });

  it("hands StatCard and Badge the same tone, unjudged as none", () => {
    expect(READING_TONES.map(statTone)).toEqual([
      "success",
      "warning",
      "danger",
      undefined,
    ]);
    expect(READING_TONES.map(badgeTone)).toEqual([
      "success",
      "warning",
      "danger",
      "default",
    ]);
  });
});

describe("a health rating and a strength band map to one tone each", () => {
  it.each([
    ["strong", "success"],
    ["good", "warning"],
    ["at_risk", "danger"],
    [undefined, "neutral"],
  ] as const)("rating %s reads %s", (rating, tone) => {
    expect(ratingTone(rating)).toBe(tone);
  });

  it.each([
    ["strong", "success"],
    ["moderate", "warning"],
    ["weak", "danger"],
    ["none", "neutral"],
  ] as const)("band %s reads %s", (band, tone) => {
    expect(bandTone(band)).toBe(tone);
  });
});
