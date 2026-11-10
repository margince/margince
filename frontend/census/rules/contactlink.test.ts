// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are source fixtures, and a template literal in them is the subject.

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { BUILDS_MAILTO, rule } from "./contactlink.census";

describe("the contactlink rule", () => {
  it("reads an interpolated mailto, and not a fixed one", () => {
    expect(BUILDS_MAILTO.test("href={`mailto:${address}`}")).toBe(true);
    expect(BUILDS_MAILTO.test('href="mailto:help@example.com"')).toBe(false);
  });

  it("reports a planted builder and the owners that build none", () => {
    const text =
      "export const F = (a: string) => <a href={`mailto:${a}`} />;\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "p.tsx"), area: "src", text },
        { path: join(srcDir, "features", "p.test.tsx"), area: "src", text },
      ]),
    ).toEqual([
      "src/features/p.tsx builds a mailto from an address",
      "src/design-system/contactlink.tsx is listed as a mailto builder and builds none",
      "src/design-system/richtext-html.ts is listed as a mailto builder and builds none",
    ]);
  });
});
