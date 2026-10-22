// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterAll, describe, expect, it } from "vitest";
import {
  type ImportGraph,
  judge,
  LEDGER,
  ownedFindings,
  ownerOf,
  parseLedger,
  reachFindings,
  readGraph,
  SRC,
  shippedInTree,
} from "./feature-boundaries.ts";

const graph = (edges: Record<string, string[]>): ImportGraph =>
  new Map(Object.entries(edges));

// A stand-in for the bundle walk: everything but tests and stories ships.
const notATest = (path: string) => !/\.(test|stories)\.tsx?$/.test(path);

describe("rule 1: only the shell imports a feature", () => {
  it("convicts a feature that imports another feature", () => {
    expect(
      reachFindings(
        graph({ "features/deals/a.tsx": ["features/leads/b.tsx"] }),
      ),
    ).toEqual(["reach features/deals/a.tsx features/leads/b.tsx"]);
  });

  it("convicts a feature that imports app/ or App", () => {
    expect(
      reachFindings(
        graph({ "features/deals/a.tsx": ["app/nav.ts", "App.tsx"] }),
      ),
    ).toEqual([
      "reach features/deals/a.tsx app/nav.ts",
      "reach features/deals/a.tsx App.tsx",
    ]);
  });

  it("convicts a test as well, since it couples the two folders too", () => {
    expect(
      reachFindings(
        graph({ "features/deals/a.test.tsx": ["features/leads/b.tsx"] }),
      ),
    ).toHaveLength(1);
  });

  it("convicts any other folder that imports a feature", () => {
    expect(
      reachFindings(
        graph({
          "mail-history/a.tsx": ["features/mail/b.tsx"],
          "components/c.tsx": ["features/deals/d.tsx"],
        }),
      ),
    ).toEqual([
      "reach mail-history/a.tsx features/mail/b.tsx",
      "reach components/c.tsx features/deals/d.tsx",
    ]);
  });

  it("lets a feature import itself and every layer below it", () => {
    expect(
      reachFindings(
        graph({
          "features/deals/deal360/a.tsx": [
            "features/deals/b.tsx",
            "components/c.tsx",
            "lib/d.ts",
            "design-system/e.tsx",
            "testing/shellharness.tsx",
          ],
        }),
      ),
    ).toEqual([]);
  });

  it("lets the shell, the test harnesses and a fork's screens import a feature", () => {
    expect(
      reachFindings(
        graph({
          "app/shell.tsx": ["features/deals/a.tsx"],
          "App.tsx": ["features/deals/a.tsx"],
          "testing/harness.tsx": ["features/deals/a.tsx"],
          "custom/warranty/screen.tsx": ["features/deals/a.tsx"],
        }),
      ),
    ).toEqual([]);
  });
});

describe("rule 2: shared code is not one feature's code", () => {
  it("convicts a shared file that one feature alone uses", () => {
    expect(
      ownedFindings(
        graph({
          "features/deals/a.tsx": ["components/c.tsx", "format/f.ts"],
          "features/deals/b.tsx": ["components/c.tsx"],
        }),
        notATest,
      ),
    ).toEqual([
      "owned components/c.tsx features/deals",
      "owned format/f.ts features/deals",
    ]);
  });

  it("clears a file two features use, or another shared file uses", () => {
    expect(
      ownedFindings(
        graph({
          "features/deals/a.tsx": ["lib/x.ts", "lib/y.ts"],
          "features/leads/b.tsx": ["lib/x.ts"],
          "lib/z.ts": ["lib/y.ts"],
        }),
        notATest,
      ),
    ).toEqual([]);
  });

  it("counts only importers that ship", () => {
    expect(
      ownedFindings(
        graph({
          "features/deals/a.tsx": ["lib/x.ts"],
          "features/leads/b.test.tsx": ["lib/x.ts"],
          "features/leads/b.stories.tsx": ["lib/x.ts"],
        }),
        notATest,
      ),
    ).toEqual(["owned lib/x.ts features/deals"]);
  });

  it("judges only components/, lib/ and format/", () => {
    expect(
      ownedFindings(
        graph({ "features/deals/a.tsx": ["design-system/x.tsx"] }),
        notATest,
      ),
    ).toEqual([]);
  });
});

describe("ownerOf", () => {
  it("names a feature by its folder and the root App files as App", () => {
    expect(ownerOf("features/deals/deal360/a.tsx")).toBe("features/deals");
    expect(ownerOf("App.views.tsx")).toBe("App");
    expect(ownerOf("app/nav.ts")).toBe("app");
  });
});

describe("the ratchet", () => {
  it("tells how to fix a new finding, in paths the move script takes", () => {
    const [failure] = judge(
      ["reach features/deals/a.tsx features/leads/b.tsx"],
      new Set(),
    );
    expect(failure).toContain(
      "src/features/deals/a.tsx imports src/features/leads/b.tsx",
    );
    expect(failure).toContain("to src/testing/ if it is a test");
    expect(failure).toContain("to src/components/ if it draws");
    expect(failure).toContain("Never add the line");
  });

  it("sends a one-feature shared file into that feature", () => {
    const [failure] = judge(["owned lib/x.ts features/deals"], new Set());
    expect(failure).toContain("Move it into src/features/deals/");
  });

  it("asks for a fixed line to be deleted", () => {
    expect(
      judge([], new Set(["reach features/a/x.ts features/b/y.ts"])),
    ).toEqual([expect.stringContaining("fixed. Delete its line")]);
  });

  it("passes when the findings and the ledger agree", () => {
    const line = "owned lib/x.ts features/deals";
    expect(judge([line], parseLedger(`# frozen\n${line}\n`))).toEqual([]);
  });

  it("refuses a malformed ledger line", () => {
    expect(() => parseLedger("reach features/a/x.ts")).toThrow(/expected/);
    expect(() => parseLedger("allow lib/x.ts features/a")).toThrow(/expected/);
  });

  it("refuses a line listed twice, since its fix would delete only one", () => {
    const line = "reach features/a/x.ts features/b/y.ts";
    expect(() => parseLedger(`${line}\n${line}\n`)).toThrow(/listed twice/);
  });
});

// The reader is what a clean tree and a blind gate share: both report nothing.
// So it is run over a planted tree that holds every form an import takes.
describe("the reader", () => {
  const root = mkdtempSync(join(tmpdir(), "feature-boundaries-"));
  afterAll(() => rmSync(root, { recursive: true, force: true }));
  const plant = (path: string, text: string) => {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), text);
  };
  plant("features/leads/b.tsx", "export const b = 1;\n");
  plant("features/leads/b.css", ".b {}\n");
  plant("features/leads/c/index.ts", "export const c = 1;\n");
  plant("features/leads/d.mts", "export const d = 1;\n");
  plant("features/leads/logo.webp", "");
  plant(
    "features/deals/a.tsx",
    [
      'import { b } from "../leads/b";',
      'import "../leads/b.css";',
      'export { c } from "../leads/c";',
      'const lazy = () => import("../leads/d.mts");',
      'import logo from "../leads/logo.webp?url";',
      '// import { gone } from "../leads/b";',
      "export const a = [b, lazy, logo];",
    ].join("\n"),
  );
  plant("features/deals/a.css", '@import "../leads/b.css";\n');
  plant("features/deals/e.jsx", 'export { b } from "../leads/b";\n');
  plant("features/deals/a.test.tsx", 'vi.mock("../leads/b");\n');

  it("sees every form of import, a mock and every kind of source file", () => {
    expect(reachFindings(readGraph(root)).sort()).toEqual([
      "reach features/deals/a.css features/leads/b.css",
      "reach features/deals/a.test.tsx features/leads/b.tsx",
      "reach features/deals/a.tsx features/leads/b.css",
      "reach features/deals/a.tsx features/leads/b.tsx",
      "reach features/deals/a.tsx features/leads/c/index.ts",
      "reach features/deals/a.tsx features/leads/d.mts",
      "reach features/deals/a.tsx features/leads/logo.webp",
      "reach features/deals/e.jsx features/leads/b.tsx",
    ]);
  });
});

describe("the tree", () => {
  const tree = readGraph();

  it("reads every feature folder", () => {
    // Derived from the folder, so a walk that stops short fails here instead
    // of reporting a smaller tree as clean.
    const features = readdirSync(join(SRC, "features"), {
      withFileTypes: true,
    })
      .filter((entry) => entry.isDirectory())
      .map((entry) => `features/${entry.name}`);
    const seen = new Set([...tree.keys()].map(ownerOf));
    expect(features.length).toBeGreaterThan(10);
    expect(features.filter((feature) => !seen.has(feature))).toEqual([]);
  });

  it("holds the rules, with only the frozen findings left", () => {
    const failures = judge(
      [...reachFindings(tree), ...ownedFindings(tree, shippedInTree())],
      parseLedger(readFileSync(LEDGER, "utf8")),
    );
    expect(failures, failures.join("\n")).toEqual([]);
  });
});
