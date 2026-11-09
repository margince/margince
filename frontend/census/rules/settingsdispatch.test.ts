// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { CARDS_THE_REGISTER_REACHED, rule } from "./settingsdispatch.census";

const dir = join(srcDir, "features", "settings");
const [kept, lost] = CARDS_THE_REGISTER_REACHED;

describe("the settingsdispatch rule", () => {
  it("follows imports inside the folder and fails a card nobody draws", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(dir, "settings.tsx"),
        area: "src",
        text: 'import { Tab } from "./tabs/tab";\nimport "../other/far";\n',
      },
      { path: join(dir, "tabs", "tab.tsx"), area: "src", text: `<${kept} />` },
      {
        path: join(srcDir, "features", "other", "far.tsx"),
        area: "src",
        text: `<${lost} />`,
      },
    ]);
    expect(findings).not.toContain(
      `${kept} is no longer reached from settings.tsx`,
    );
    expect(findings).toContain(
      `${lost} is no longer reached from settings.tsx`,
    );
  });
});
