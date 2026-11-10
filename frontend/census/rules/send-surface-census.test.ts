// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { rule, silentSenders } from "./send-surface-census.census";

const at = (name: string, text: string) => ({
  path: join(srcDir, "features", name),
  area: "src" as const,
  text,
});

describe("the send-surface-census census", () => {
  it("honours a waiver and reports one whose file is gone", () => {
    const tree = treeOf([at("bulk.tsx", 'api.POST("/emails", {});')]);
    expect(
      silentSenders(rule.corpus(tree), {
        "src/features/bulk.tsx": "the drawer beside it shows the mark",
        "src/features/gone.tsx": "a surface nobody can find",
      }),
    ).toEqual(["src/features/gone.tsx is in SILENT_SENDERS and is gone"]);
  });
});

describe("the send-surface-census rule", () => {
  it("reports a sender that draws no mark, and passes one that does", () => {
    expect(
      plantedFindings(rule, [
        at("silent.tsx", 'api.POST("/activities/{id}/send-email", {});'),
        at("marked.tsx", 'api.POST("/emails", {});\n<SendMark />;'),
        at("silent.test.tsx", 'api.POST("/emails", {});'),
      ]),
    ).toEqual(["src/features/silent.tsx posts a send and draws no mark"]);
  });
});
