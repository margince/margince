// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./document-list-keys.census";

const at = (...parts: string[]) => join(srcDir, ...parts);

describe("the document-list-keys rule", () => {
  it("fails every spelling of a list key outside the owner, tests included", () => {
    expect(
      plantedFindings(rule, [
        {
          path: at("lib", "attachmentkeys.ts"),
          area: "src",
          text: 'export const P = ["companyDocuments"] as const;\n',
        },
        {
          path: at("features", "double.ts"),
          area: "src",
          text: 'const key = ["attachments", "contact", id];\n',
        },
        {
          path: at("features", "single.ts"),
          area: "src",
          text: "const key = ['deal-room-documents', id];\n",
        },
        {
          path: at("features", "template.ts"),
          area: "src",
          text: "const key = [`attachments`, `activity`, id];\n",
        },
        {
          path: at("features", "spread.test.tsx"),
          area: "src",
          text: 'const key = [\n  "deal-documents",\n  id,\n] as const;\n',
        },
        {
          path: at("features", "clean.ts"),
          area: "src",
          text: [
            'const compose = ["attachments", kind, id, "compose"];',
            "// a comment naming companyDocuments",
            'const label = "deal-documents";',
            "",
          ].join("\n"),
        },
      ]),
    ).toEqual([
      "src/features/double.ts:1 spells a file list's query key by hand",
      "src/features/single.ts:1 spells a file list's query key by hand",
      "src/features/template.ts:1 spells a file list's query key by hand",
      "src/features/spread.test.tsx:1 spells a file list's query key by hand",
    ]);
  });
});
