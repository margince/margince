// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./settings-imports.census";

const at = (rel: string, text = "") => ({
  path: join(srcDir, rel),
  area: "src" as const,
  text,
});

describe("the settings-imports rule", () => {
  it("fails the nav and a screen that reach the settings screen", () => {
    const findings = plantedFindings(rule, [
      at("features/settings/settings.tsx", 'import "./settingsnav";'),
      at("features/settings/settingsnav.tsx", 'import "./somecard";'),
      at("features/settings/somecard.tsx", 'import "./settings";'),
      at("features/settings/somecard.test.tsx", 'import "./settings";'),
      at("features/deals/deal.tsx"),
    ]);
    expect(findings).toEqual([
      "reaches settings.tsx: src/features/settings/settingsnav.tsx -> src/features/settings/somecard.tsx -> src/features/settings/settings.tsx",
      "reaches settings.tsx: src/features/settings/somecard.tsx -> src/features/settings/settings.tsx",
    ]);
  });
});
