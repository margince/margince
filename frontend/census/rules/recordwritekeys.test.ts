// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./recordwritekeys.census";

describe("the recordwritekeys rule", () => {
  it("fails a hand-written restore and spares the helper and stories", () => {
    const byHand =
      'h({ onRestored: () => client.invalidateQueries({ queryKey: ["deal", id] }) });\n';
    const dir = join(srcDir, "features", "deals");
    const findings = plantedFindings(rule, [
      { path: join(dir, "dealpage.tsx"), area: "src", text: byHand },
      { path: join(dir, "dealpage.stories.tsx"), area: "src", text: byHand },
      {
        path: join(dir, "dealrail.tsx"),
        area: "src",
        text: 'h({ onRestored: () => invalidateRecord(client, "deal", id) });\n',
      },
    ]);
    expect(findings).toEqual([
      'src/features/deals/dealpage.tsx: () => client.invalidateQueries({ queryKey: ["deal", id] })',
    ]);
  });
});

describe("the onRestored prefilter", () => {
  it("reads a hand invalidation however the callback is spelled", () => {
    const at = (name: string) =>
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "plant", "a.ts"),
          area: "src",
          text: `const o = { ${name}: () => qc.invalidateQueries({ queryKey: ["x"] }) };\n`,
        },
      ]);
    expect(at("onRestored")).toHaveLength(1);
    expect(at("on\\u0052estored")).toHaveLength(1);
  });
});
