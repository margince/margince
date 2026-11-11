// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./problem-words.census";

const imports =
  'import { problemFieldErrorsOf, problemFieldSentencesOf } from "../lib/problem";\n' +
  'import type { components } from "../api/schema";\n';

const planted = (name: string, body: string) => ({
  path: join(srcDir, "features", name),
  area: "src" as const,
  text: imports + body,
});

describe("the problem-words rule", () => {
  it("finds every way a screen prints the server's words, and nothing else", () => {
    const findings = plantedFindings(rule, [
      planted(
        "read.ts",
        "export const a = (e: unknown) => problemFieldErrorsOf(e).map((p) => [p.field, p.message]);\n",
      ),
      planted(
        "destructure.ts",
        "export const b = (e: unknown) => problemFieldErrorsOf(e).map(({ field, message }) => field + message);\n",
      ),
      planted(
        "contract.ts",
        'export const c = (p: components["schemas"]["Problem"]) => p.detail;\n',
      ),
      planted(
        "sound.ts",
        "export const d = (e: unknown) => problemFieldErrorsOf(e).map((p) => p.code);\n" +
          'export const f = (e: unknown) => problemFieldSentencesOf(e, "de");\n' +
          "export const g = (e: Error) => e.message;\n",
      ),
      {
        path: join(srcDir, "features", "read.test.ts"),
        area: "src",
        text: `${imports}export const h = (e: unknown) => problemFieldErrorsOf(e)[0]?.message;\n`,
      },
    ]);
    expect(findings).toEqual([
      "src/features/read.ts:3 prints the server's words for a refusal",
      "src/features/destructure.ts:3 prints the server's words for a refusal",
      "src/features/contract.ts:3 prints the server's words for a refusal",
    ]);
  });
});
