// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { misfiling, rule } from "./settingsstories.census";

const story = (name: string, title: string) => ({
  path: join(srcDir, "features", "settings", `${name}.stories.tsx`),
  area: "src" as const,
  text: `export default { title: "${title}" };\n`,
});

describe("the settingsstories rule", () => {
  it("passes the surfaces and the listed cross-page stories", () => {
    expect(misfiling("Settings/Settings screen")).toBeNull();
    expect(misfiling("Settings/Across pages/Refresh from sources")).toBeNull();
  });

  it("fails an unlisted cross-page story, a short title and an unknown page", () => {
    expect(misfiling("Settings/Across pages/New thing")).toMatch(
      /not listed in ACROSS_PAGE_STORIES/,
    );
    expect(misfiling("Settings/Nonsense")).toMatch(/is not Settings\/<Group>/);
    expect(misfiling("Settings/Admin settings/Old tab/Card")).toMatch(
      /does not declare/,
    );
  });

  it("reads titles off the default export and fails a misfiled planted story", () => {
    const findings = plantedFindings(rule, [
      story("old", "Settings/Admin settings/Old tab/Card"),
      story("surface", "Settings/Settings screen"),
      story("other", "Deals/Board"),
    ]);
    expect(findings).toEqual([
      'src/features/settings/old.stories.tsx: "Settings/Admin settings/Old tab/Card" files under Admin settings/Old tab, which the catalog does not declare',
    ]);
  });
});
