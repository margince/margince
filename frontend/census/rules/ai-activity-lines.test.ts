// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./ai-activity-lines.census";

const screen = (text: string) => ({
  path: join(srcDir, "features", "planted.tsx"),
  area: "src" as const,
  text,
});

describe("the ai-activity-lines rule", () => {
  it("passes a screen with one draft key and one send key", () => {
    const text =
      'useMutation({ mutationKey: ["email-draft", id] });\n' +
      'useMutation({ mutationKey: ["email", id] });\n';
    expect(plantedFindings(rule, [screen(text)])).toEqual([]);
  });

  it("reports a draft renamed back onto the send key", () => {
    const text = 'useMutation({ mutationKey: ["email", id] });\n'.repeat(2);
    expect(plantedFindings(rule, [screen(text)])).toEqual([
      'src/features/planted.tsx spells ["email", …] 2 times',
    ]);
  });
});
