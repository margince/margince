// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { KEY_LITERAL, rule } from "./companyfactspanel.census";

describe("the companyfactspanel rule", () => {
  it("fails a planted copy of the key and spares the owner", () => {
    const text = `useQuery({ queryKey: [${KEY_LITERAL}, id] });\n`;
    const dir = join(srcDir, "features", "companies");
    const findings = plantedFindings(rule, [
      { path: join(dir, "companyfactspanel.tsx"), area: "src", text },
      { path: join(dir, "crawl.ts"), area: "src", text },
      { path: join(dir, "crawl.test.ts"), area: "src", text: "factsKey(id);" },
    ]);
    expect(findings).toEqual([
      "src/features/companies/crawl.ts: spells the company-facts key itself",
    ]);
  });
});
