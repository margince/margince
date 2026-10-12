// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  type Module,
  moduleGraph,
  reachedModules,
  SRC_ROOT,
} from "../../scripts/lib/bundle-reach";
import { en } from "./en";
import { LOCALES } from "./index";

// A key is live only when a module the shipped bundle loads names it: a story,
// a test, a fixture or a module nothing imports vouches for nothing.

/**
 * Keys no loaded module renders, literally, under a template stem, or as the
 * arm of a plural pair whose base it names.
 */
function orphanKeys(
  keys: readonly string[],
  modules: Iterable<Module>,
): string[] {
  const loaded = [...modules];
  const literals = new Set(loaded.flatMap((module) => module.literals));
  const stems = [...new Set(loaded.flatMap((module) => module.stems))];
  const catalog = new Set(keys);
  // Only the two arms of a full pair: half a pair is the orphan this finds.
  const underPluralBase = (key: string): boolean => {
    const arm = ["_one", "_other"].find((suffix) => key.endsWith(suffix));
    if (arm === undefined) return false;
    const base = key.slice(0, -arm.length);
    return (
      catalog.has(`${base}_one`) &&
      catalog.has(`${base}_other`) &&
      literals.has(base)
    );
  };
  return keys.filter(
    (key) =>
      !literals.has(key) &&
      !underPluralBase(key) &&
      !stems.some((stem) => key.startsWith(stem)),
  );
}

describe("the orphan finder, over a planted tree", () => {
  const MAIN = "/app/src/main.tsx";

  const verdict = (
    files: Record<string, string>,
    keys: readonly string[],
  ): string[] => {
    const graph = moduleGraph({
      tree: new Map(Object.entries(files)),
      aliases: new Map([["@alias/copy", "/app/src/aliased"]]),
      catalogs: new Set(),
    });
    return orphanKeys(keys, graph.reach([MAIN]).values());
  };

  it("counts a key the entry renders", () => {
    expect(verdict({ [MAIN]: `t("a.live");` }, ["a.live"])).toEqual([]);
  });

  it("refuses a key only a story renders", () => {
    const files = {
      [MAIN]: `import { Card } from "./card";`,
      "/app/src/card.tsx": `export const Card = () => null;`,
      "/app/src/card.stories.tsx": `import { Card } from "./card"; t("a.story");`,
    };
    expect(verdict(files, ["a.story"])).toEqual(["a.story"]);
  });

  it("refuses a key only an erased type import reaches", () => {
    const files = {
      [MAIN]: [
        `import type { A } from "./a";`,
        `export type { C } from "./c";`,
        `import { type B } from "./b";`,
      ].join("\n"),
      "/app/src/a.ts": `export type A = 1; t("a.typeImport");`,
      "/app/src/c.ts": `export type C = 1; t("c.typeExport");`,
      "/app/src/b.ts": `export type B = 1; t("b.inlineType");`,
    };
    const keys = ["a.typeImport", "c.typeExport", "b.inlineType"];
    expect(verdict(files, keys)).toEqual(["a.typeImport", "c.typeExport"]);
  });

  it("refuses a key named only as a type or a property name", () => {
    const files = {
      [MAIN]: [
        `type K = "a.asType";`,
        `const table = { "a.asName": 1, b: "a.asValue" };`,
      ].join("\n"),
    };
    const keys = ["a.asType", "a.asName", "a.asValue"];
    expect(verdict(files, keys)).toEqual(["a.asType", "a.asName"]);
  });

  it("refuses a key only a module nothing imports renders", () => {
    const files = { [MAIN]: ``, "/app/src/stray.tsx": `t("a.stray");` };
    expect(verdict(files, ["a.stray"])).toEqual(["a.stray"]);
  });

  it("refuses a key only a comment names", () => {
    const files = { [MAIN]: `// t("a.line")\n/* "a.block" */ t("a.live");` };
    expect(verdict(files, ["a.line", "a.block", "a.live"])).toEqual([
      "a.line",
      "a.block",
    ]);
  });

  it("takes template stems only from loaded modules", () => {
    const files = {
      [MAIN]: `t(\`live.\${status}\`);`,
      "/app/src/stray.tsx": `t(\`stray.\${status}\`);`,
    };
    expect(verdict(files, ["live.open", "stray.open"])).toEqual(["stray.open"]);
  });

  it("follows a dynamic import, a re-export chain, an alias and a glob", () => {
    const files = {
      [MAIN]: [
        `const Lazy = lazy(() => import("./lazy"));`,
        `import { Deep } from "./barrel";`,
        `import copy from "@alias/copy";`,
        `const screens = import.meta.glob("./custom/*/screen.tsx");`,
      ].join("\n"),
      "/app/src/lazy.tsx": `t("a.lazy");`,
      "/app/src/barrel/index.ts": `export * from "./middle.ts";`,
      "/app/src/barrel/middle.ts": `export { Deep } from "../deep";`,
      "/app/src/deep.tsx": `t("a.reexported");`,
      "/app/src/aliased.ts": `t("a.aliased");`,
      "/app/src/custom/one/screen.tsx": `t("a.globbed");`,
    };
    const keys = ["a.lazy", "a.reexported", "a.aliased", "a.globbed"];
    expect(verdict(files, keys)).toEqual([]);
  });

  it("does not run a module imported as text or a URL, but runs a worker", () => {
    const files = {
      [MAIN]: [
        `import raw from "./raw.ts?raw";`,
        `import url from "./url.ts?url";`,
        `import Worker from "./work.ts?worker";`,
      ].join("\n"),
      "/app/src/raw.ts": `t("a.raw");`,
      "/app/src/url.ts": `t("a.url");`,
      "/app/src/work.ts": `t("a.worker");`,
    };
    const keys = ["a.raw", "a.url", "a.worker"];
    expect(verdict(files, keys)).toEqual(["a.raw", "a.url"]);
  });

  it("counts a plural pair by its base only where a loaded module names it", () => {
    const files = {
      [MAIN]: `plural("live.count", n);`,
      "/app/src/main.test.ts": `plural("dead.count", n);`,
    };
    const keys = [
      "live.count_one",
      "live.count_other",
      "dead.count_one",
      "dead.count_other",
    ];
    expect(verdict(files, keys)).toEqual([
      "dead.count_one",
      "dead.count_other",
    ]);
  });

  it("does not stretch a plural base past its two arms", () => {
    const files = { [MAIN]: `plural("a.count", n);` };
    const keys = ["a.count_one", "a.count_other", "a.count_extra"];
    expect(verdict(files, keys)).toEqual(["a.count_extra"]);
  });
});

describe("catalog keys against the bundle that renders them", () => {
  const catalogs = new Set(
    LOCALES.map((locale) => join(SRC_ROOT, "i18n", `${locale}.ts`)),
  );
  const mounted = reachedModules({ catalogs });
  const keys = Object.keys(en);

  it("every key is rendered by a module the bundle loads", () => {
    const orphans = orphanKeys(keys, mounted.values());
    expect(
      orphans,
      `keys translated three times and rendered nowhere: ${orphans.join(", ")}`,
    ).toEqual([]);
  });
});
