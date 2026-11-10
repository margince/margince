// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, repoRoot, srcDir } from "../tree";
import {
  FIXTURE,
  mayReachForStorage,
  OWNER,
  rule,
  storageReachesIn,
} from "./storage-spelling.census";

// One reach per line, in every shape the tree has used or a rename would take.
const PLANTED_SPELLINGS = [
  "window.localStorage.getItem(key);",
  "globalThis.sessionStorage?.setItem(key, value);",
  "localStorage.removeItem(key);",
  "const store = sessionStorage;",
  'const bracketed = window["localStorage"];',
  "const { localStorage } = window;",
  "const { sessionStorage: store } = globalThis;",
  'const { "localStorage": quoted } = window;',
  'if ("sessionStorage" in window) run();',
  "type Kept = typeof localStorage;",
] as const;

// Mentions that are not a reach: a comment, and a name that only starts alike.
const PLANTED_INNOCENTS = [
  "// localStorage in a comment",
  "/** sessionStorage in a doc comment */",
  "const localStorageish = readStored(STORAGE_KEYS.theme);",
] as const;

describe("the storage-spelling rule", () => {
  it("finds the reach however the caller spells it", () => {
    const planted = parseSource(
      "planted.tsx",
      [...PLANTED_SPELLINGS, ...PLANTED_INNOCENTS].join("\n"),
    );
    expect(storageReachesIn(planted)).toEqual(
      PLANTED_SPELLINGS.map((_, index) => index + 1),
    );
  });

  it("fails a planted file, and spares only the owner and fixtures", () => {
    const text = `// a comment\n${PLANTED_SPELLINGS[0]}\n`;
    const findings = plantedFindings(rule, [
      { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
      { path: join(srcDir, "features", "planted.test.tsx"), area: "src", text },
      { path: join(repoRoot, OWNER), area: "src", text },
    ]);
    expect(findings).toEqual(["frontend/src/features/planted.tsx:2"]);
    expect(FIXTURE.test("shell.test.tsx")).toBe(true);
    expect(FIXTURE.test("buyerroom.stories.tsx")).toBe(true);
    expect(FIXTURE.test("buyerroomsession.ts")).toBe(false);
  });

  it("lets through every shape the matcher can see", () => {
    for (const spelling of PLANTED_SPELLINGS) {
      expect(mayReachForStorage(spelling)).toBe(true);
    }
  });
});
