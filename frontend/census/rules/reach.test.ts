// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { doorsIn, routerModule, rule, waiversIn } from "./reach.census";

const modules = (...planted: (readonly [string, string])[]) =>
  treeOf(
    planted.map(([name, text]) => ({
      path: join(srcDir, name),
      area: "src" as const,
      text,
    })),
  ).files;

describe("the reach census", () => {
  it("sees a door in every spelling a door takes", () => {
    const screens = new Set(["planted", "other"]);
    const linkedBy = (source: string) =>
      doorsIn(modules(["planted.ts", source]), screens);

    expect(linkedBy(`const href = "#/planted";`)).toContain("planted");
    expect(linkedBy(`const href = \`#/planted/\${id}\`;`)).toContain("planted");
    // The address in a template span, which is how a link from an origin looks.
    expect(
      linkedBy(`const href = \`\${origin}/#/planted?c=\${token}\`;`),
    ).toContain("planted");
    // The origin written out, as a mailed address spells it.
    expect(linkedBy(`const href = "https://app.example/#/planted";`)).toContain(
      "planted",
    );
    expect(linkedBy(`navigate({ screen: "planted" });`)).toContain("planted");
    // A route object held rather than navigated to: the palette's shape.
    expect(linkedBy(`const row = { route: { screen: "planted" } };`)).toContain(
      "planted",
    );
    expect(linkedBy(`const label = "planted";`).size).toBe(0);
    // Prose that tells a reader to type an address is no door.
    expect(linkedBy(`const help = "Open #/planted to see it";`).size).toBe(0);
    // A set of screens describes how they are framed; it links nowhere.
    expect(
      linkedBy(`const railLess = new Set(["planted", "other"]);`).size,
    ).toBe(0);
  });

  // In both orders, since a walk that resolved a name on sight would answer by
  // file order.
  it("follows a door written through an imported constant, either file first", () => {
    const screens = new Set(["planted"]);
    const declares = [
      "a.ts",
      `export const PLANTED_SCREEN = "planted";`,
    ] as const;
    const uses = [
      "b.ts",
      `import { PLANTED_SCREEN } from "./a";\nnavigate({ screen: PLANTED_SCREEN });`,
    ] as const;

    expect(doorsIn(modules(declares, uses), screens)).toContain("planted");
    expect(doorsIn(modules(uses, declares), screens)).toContain("planted");
    const renamed = [
      "c.ts",
      `import { PLANTED_SCREEN as HOME } from "./a";\nnavigate({ screen: HOME });`,
    ] as const;
    expect(doorsIn(modules(declares, renamed), screens)).toContain("planted");
    expect(doorsIn(modules(uses), screens).size).toBe(0);
  });

  it("does not let an unrelated file's same-named constant open a door", () => {
    const screens = new Set(["planted"]);
    const elsewhere = ["elsewhere.ts", `const SCREEN_ID = "planted";`] as const;
    const uses = ["uses.ts", `navigate({ screen: SCREEN_ID });`] as const;

    expect(doorsIn(modules(elsewhere, uses), screens).size).toBe(0);
    const packaged = [
      "packaged.ts",
      `import { SCREEN_ID } from "some-package";\nnavigate({ screen: SCREEN_ID });`,
    ] as const;
    expect(doorsIn(modules(elsewhere, packaged), screens).size).toBe(0);
  });

  it("reads a name in its own scope, not a same-named one beside it", () => {
    const screens = new Set(["planted", "other"]);
    const linkedBy = (source: string) =>
      doorsIn(modules(["scoped.ts", source]), screens);
    const twoLocals = (opened: string, beside: string) =>
      [
        `function opens() { const id = "${opened}"; navigate({ screen: id }); }`,
        `function names() { const id = "${beside}"; return id; }`,
      ].join("\n");

    expect([...linkedBy(twoLocals("planted", "other"))]).toEqual(["planted"]);
    expect([...linkedBy(twoLocals("other", "planted"))]).toEqual(["other"]);
    // A parameter shadows the module's constant of the same name.
    expect(
      linkedBy(
        [
          `const id = "planted";`,
          `function opens(id: string) { navigate({ screen: id }); }`,
        ].join("\n"),
      ).size,
    ).toBe(0);
  });

  it("does not accept a waiver with no reason", () => {
    const waived = waiversIn(
      parseSource(
        "router.tsx",
        [
          `export const SCREENS = [`,
          `  // reach:external opened by the browser extension, never linked here`,
          `  "client",`,
          `  // reach:external`,
          `  "bare",`,
          `  "plain",`,
          `] as const;`,
        ].join("\n"),
      ),
    );

    expect(waived.get("client")).toBe(
      "opened by the browser extension, never linked here",
    );
    expect(waived.get("bare")).toBeNull();
    expect(waived.get("plain")).toBeNull();
  });
});

describe("the reach rule", () => {
  // The real router beside planted modules, so the imported list and the one
  // read from source agree.
  const router = {
    path: routerModule,
    area: "src" as const,
    text: readFileSync(routerModule, "utf8"),
  };

  it("reports a planted screen with no door, and drops it once linked", () => {
    const unreached = plantedFindings(rule, [router]);
    const [screen] = unreached;
    expect(screen).toBeDefined();
    const linked = plantedFindings(rule, [
      router,
      {
        path: join(srcDir, "features", "planted.ts"),
        area: "src",
        text: `const href = "#/${screen}";`,
      },
    ]);
    expect(linked).not.toContain(screen);
  });

  it("reports a router whose source list drifts from the imported one", () => {
    const drifted = {
      ...router,
      text: `export const SCREENS = ["x"] as const;`,
    };
    expect(plantedFindings(rule, [drifted])).toEqual([
      "router.tsx's SCREENS list, read from source, differs from the imported one",
    ]);
  });
});
