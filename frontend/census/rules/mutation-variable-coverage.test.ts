// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { findingsIn, rule } from "./mutation-variable-coverage.census";

const found = (text: string) =>
  findingsIn("planted.tsx", parseSource("planted.tsx", text));

describe("the mutation-variable-coverage census", () => {
  it("sees a guard on a closed-over value, through a property read", () => {
    expect(
      found(
        'useMutation({ mutationFn: async () => { if (!form.name) throw new Error("x"); } });',
      ),
    ).toHaveLength(1);
    expect(
      found(
        "useMutation({ mutationFn: () => { if (!draft) throwProblem(t); } });",
      ),
    ).toHaveLength(1);
  });

  it("leaves a variable, a local and a guard that does not refuse alone", () => {
    const allowed = [
      "useMutation({ mutationFn: async (form) => { if (!form) throw new Error(); } });",
      "useMutation({ mutationFn: async () => { const { data } = await get(); if (!data) throw new Error(); } });",
      "useMutation({ mutationFn: async () => { if (!form) return; } });",
    ];
    for (const text of allowed) {
      expect(found(text), text).toEqual([]);
    }
  });
});

describe("the mutation-variable-coverage rule", () => {
  it("reports a planted closed-over guard through its own corpus and visit", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text =
      "useMutation({\n  mutationFn: async () => {\n    if (!form) throw new Error();\n  },\n});\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "src/features/planted.tsx:3 — mutationFn takes no variable and refuses on closed-over `form`; pass it to mutate() instead",
    ]);
  });
});
