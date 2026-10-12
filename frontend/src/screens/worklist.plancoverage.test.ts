// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { planCoverageGap, planCoverageText } from "./worklist.plancoverage";

// "Nothing due" and "not looked at" differ. A lead reading a team's day must be
// told whose weekly plans it covered, and by name whose it did not.

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

function teammate(display_name: string, read = true) {
  return { user_id: `id-${display_name}`, display_name, read };
}

const five = ["Ana", "Ben", "Cara", "Dev", "Eve"].map((name) => teammate(name));

describe("whose weekly plans a team read covered", () => {
  it("says every plan was read when every plan was", () => {
    const coverage = { members: five, truncated: false };

    expect(planCoverageText(coverage, "en", t)).toBe(
      "Weekly plans read for all 5 teammates.",
    );
    expect(planCoverageGap(coverage)).toBe(false);
  });

  it("names the teammates whose plans were not read", () => {
    const coverage = {
      members: [
        teammate("Ana", false),
        ...five.slice(1, 4),
        teammate("Eve", false),
      ],
      truncated: false,
    };

    expect(planCoverageText(coverage, "en", t)).toBe(
      "Weekly plans read for 3 of 5 teammates. Not read: Ana and Eve.",
    );
    expect(planCoverageGap(coverage)).toBe(true);
  });

  // "All" would contradict a team list cut short, so the figures are a share.
  it("admits a cut team list rather than claiming all", () => {
    const coverage = { members: five, truncated: true };

    expect(planCoverageText(coverage, "en", t)).toBe(
      "Weekly plans read for 5 of 5 teammates. The team list was cut short, so later teammates were not checked.",
    );
    expect(planCoverageGap(coverage)).toBe(true);
  });

  it("says nothing when no team read was made, or its roster was empty", () => {
    expect(planCoverageText(undefined, "en", t)).toBeNull();
    expect(
      planCoverageText({ members: [], truncated: false }, "en", t),
    ).toBeNull();
    expect(planCoverageGap(undefined)).toBe(false);
  });

  it("words one teammate in the reader's plural", () => {
    expect(
      planCoverageText(
        { members: [teammate("Ana")], truncated: false },
        "de",
        (key, params) => translate("de", key, params),
      ),
    ).toBe("Wochenplan von einem Teammitglied gelesen.");
  });
});
