// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll, describe, expect, it } from "vitest";
import { testCosts } from "../../scripts/lib/test-cost";
import { plantedSettled, treeOf } from "../tree";
import { missingKinds, outsideTheProject, rule } from "./vitest-shards.census";

// testCosts reads each file and its imports by path, so the planted tests are
// written to a temporary directory of their own.
describe("the vitest-shards rule", () => {
  const dir = mkdtempSync(join(tmpdir(), "vitest-shards-"));
  afterAll(() => rmSync(dir, { recursive: true, force: true }));
  const plant = (name: string, text: string) => {
    const path = join(dir, name);
    writeFileSync(path, text);
    return { path, area: "src" as const, text };
  };
  const render = plant("render.test.ts", 'import { expect } from "vitest";\n');
  const parse = plant(
    "parse.test.ts",
    'import ts from "typescript";\nts.SyntaxKind;\n',
  );

  it("reports the kinds a suite of render tests lacks", () => {
    expect(missingKinds(testCosts([render.path]))).toEqual(["parse"]);
    expect(missingKinds(testCosts([render.path, parse.path]))).toEqual([]);
  });

  it("reports a compiler-loading file the unisolated project leaves out", () => {
    const { files } = treeOf([render, parse]);
    const costs = testCosts(files.map((file) => file.path));
    const [, parsed] = files;
    expect(outsideTheProject(files, costs, new Set())).toEqual([parsed.rel]);
    expect(outsideTheProject(files, costs, new Set([parsed.rel]))).toEqual([]);
  });

  it("reports a planted parse test through its own corpus and settle", async () => {
    const findings = await plantedSettled(rule, [render, parse]);
    expect(findings).toHaveLength(1);
    expect(findings[0]).toMatch(
      /parse\.test\.ts loads the compiler and is not in the unisolated project$/,
    );
  });
});
