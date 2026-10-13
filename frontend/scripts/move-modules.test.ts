import { describe, expect, it } from "vitest";
import { planMoves, rewriteImports } from "./move-modules";

const files = new Set([
  "/fe/src/app/storage.ts",
  "/fe/src/app/storage.test.ts",
  "/fe/src/app/shell.tsx",
  "/fe/src/api/client.ts",
  "/fe/src/screens/common.tsx",
  "/fe/src/screens/common.css",
  "/fe/src/screens/deal/index.ts",
  "/fe/src/screens/deal/card.tsx",
]);
const exists = (path: string) => files.has(path);
const moves = new Map([
  ["/fe/src/app/storage.ts", "/fe/src/lib/storage.ts"],
  ["/fe/src/app/storage.test.ts", "/fe/src/lib/storage.test.ts"],
]);

describe("moving modules", () => {
  it("keeps the layout under a moved directory", () => {
    const planned = planMoves(
      [["/fe/src/screens/deal", "/fe/src/features/deals"]],
      () => ["/fe/src/screens/deal/index.ts", "/fe/src/screens/deal/card.tsx"],
      () => true,
    );
    expect([...planned.values()]).toEqual([
      "/fe/src/features/deals/index.ts",
      "/fe/src/features/deals/card.tsx",
    ]);
  });

  it("refuses two files moving to one place", () => {
    expect(() =>
      planMoves(
        [
          ["/fe/a.ts", "/fe/c.ts"],
          ["/fe/b.ts", "/fe/c.ts"],
        ],
        () => [],
        () => false,
      ),
    ).toThrow(/two files would move to \/fe\/c\.ts/);
  });

  it("points an import at the moved file, in every specifier form", () => {
    const text = [
      'import { readKey } from "../app/storage";',
      'export type { Key } from "../app/storage";',
      'const lazy = () => import("../app/storage");',
      'vi.mock("../app/storage", () => ({}));',
    ].join("\n");
    expect(rewriteImports("/fe/src/api/client.ts", text, moves, exists)).toBe(
      [
        'import { readKey } from "../lib/storage";',
        'export type { Key } from "../lib/storage";',
        'const lazy = () => import("../lib/storage");',
        'vi.mock("../lib/storage", () => ({}));',
      ].join("\n"),
    );
  });

  it("re-spells the imports inside a moved file", () => {
    const text =
      'import { shell } from "./shell";\nimport { k } from "./storage";';
    expect(
      rewriteImports("/fe/src/app/storage.test.ts", text, moves, exists),
    ).toBe(
      'import { shell } from "../app/shell";\nimport { k } from "./storage";',
    );
  });

  it("keeps an extension, a directory index and a stylesheet as written", () => {
    const deal = new Map([
      ["/fe/src/screens/deal/index.ts", "/fe/src/features/deals/index.ts"],
    ]);
    const css = new Map([
      ["/fe/src/screens/common.css", "/fe/src/lib/common.css"],
    ]);
    expect(
      rewriteImports(
        "/fe/src/app/shell.tsx",
        'import "../app/storage.ts";',
        moves,
        exists,
      ),
    ).toBe('import "../lib/storage.ts";');
    expect(
      rewriteImports(
        "/fe/src/app/shell.tsx",
        'import { d } from "../screens/deal";',
        deal,
        exists,
      ),
    ).toBe('import { d } from "../features/deals";');
    expect(
      rewriteImports(
        "/fe/src/screens/common.tsx",
        'import "./common.css";',
        css,
        exists,
      ),
    ).toBe('import "../lib/common.css";');
  });

  it("leaves package imports and unmoved paths alone", () => {
    const text =
      'import React from "react";\nimport { c } from "../screens/common";';
    expect(rewriteImports("/fe/src/app/shell.tsx", text, moves, exists)).toBe(
      text,
    );
  });
});
