// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { moduleGraph } from "../../scripts/lib/bundle-reach";
import { liveKeyUse } from "../../scripts/lib/key-reach";
import { stable } from "../../src/format/collate";
import { en } from "../../src/i18n/en";
import { plantedFindings, srcDir } from "../tree";
import { orphanKeys, rule } from "./orphan-keys.census";

describe("the orphan finder, over a planted tree", () => {
  const MAIN = "/app/src/main.tsx";

  // No lib: the cases need literal unions and symbols, not the DOM.
  const programOver = (files: Record<string, string>): ts.Program => {
    const options: ts.CompilerOptions = {
      noLib: true,
      noEmit: true,
      strict: true,
      jsx: ts.JsxEmit.Preserve,
      module: ts.ModuleKind.ESNext,
      moduleResolution: ts.ModuleResolutionKind.Bundler,
      types: [],
      baseUrl: "/",
      paths: { "@alias/copy": ["/app/src/aliased"] },
    };
    const host = ts.createCompilerHost(options);
    host.getSourceFile = (name) =>
      name in files
        ? ts.createSourceFile(name, files[name], ts.ScriptTarget.ESNext, true)
        : undefined;
    host.fileExists = (name) => name in files;
    host.readFile = (name) => files[name];
    host.directoryExists = () => true;
    return ts.createProgram(Object.keys(files), options, host);
  };

  const verdict = (
    files: Record<string, string>,
    keys: readonly string[],
  ): string[] => {
    const catalogs = new Set<string>();
    const graph = moduleGraph({
      tree: new Map(Object.entries(files)),
      aliases: new Map([["@alias/copy", "/app/src/aliased"]]),
      catalogs,
    });
    const loaded = graph.reach([MAIN]);
    return orphanKeys(
      keys,
      liveKeyUse(programOver(files), loaded, [MAIN], catalogs),
    );
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
        `export const table = { "a.asName": 1, b: "a.asValue" };`,
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

  it("refuses a key only an export nothing renders names", () => {
    const files = {
      [MAIN]: `import { Used } from "./card"; export const page = <Used />;`,
      "/app/src/card.tsx": [
        `export const Used = () => t("a.used");`,
        `export const Unused = () => t("a.unused");`,
      ].join("\n"),
    };
    expect(verdict(files, ["a.used", "a.unused"])).toEqual(["a.unused"]);
  });

  it("follows a name through a re-export, an alias and a helper", () => {
    const files = {
      [MAIN]: [
        `import { Deep as Renamed } from "./barrel";`,
        `export const page = <Renamed />;`,
      ].join("\n"),
      "/app/src/barrel.ts": `export * from "./deep";`,
      "/app/src/deep.tsx": [
        `const label = () => t("a.helper");`,
        `export const Deep = () => label();`,
        `export const Other = () => t("a.other");`,
      ].join("\n"),
    };
    expect(verdict(files, ["a.helper", "a.other"])).toEqual(["a.other"]);
  });

  it("counts what a loaded module does when it loads", () => {
    const files = {
      [MAIN]: `import "./setup";`,
      "/app/src/setup.ts": [
        `register(t("a.effect"));`,
        `export const table = register(t("a.exported"));`,
        `export const never = () => t("a.never");`,
      ].join("\n"),
    };
    expect(verdict(files, ["a.effect", "a.exported", "a.never"])).toEqual([
      "a.never",
    ]);
  });

  it("counts every export of a module read by import(), a glob or a namespace", () => {
    const files = {
      [MAIN]: [
        `const Lazy = lazy(() => import("./lazy"));`,
        `import * as all from "./all";`,
        `const screens = import.meta.glob("./custom/*.tsx");`,
        `export const page = [Lazy, all, screens];`,
      ].join("\n"),
      "/app/src/lazy.tsx": `export const Screen = () => t("a.lazy");`,
      "/app/src/all.ts": `export const One = () => t("a.namespace");`,
      "/app/src/custom/one.tsx": `export const Two = () => t("a.globbed");`,
    };
    expect(verdict(files, ["a.lazy", "a.namespace", "a.globbed"])).toEqual([]);
  });

  it("honours a glob's negated pattern", () => {
    const files = {
      [MAIN]: `const s = import.meta.glob(["./custom/*.tsx", "!./custom/skip.tsx"]);`,
      "/app/src/custom/keep.tsx": `export const A = () => t("a.kept");`,
      "/app/src/custom/skip.tsx": `export const B = () => t("a.skipped");`,
    };
    expect(verdict(files, ["a.kept", "a.skipped"])).toEqual(["a.skipped"]);
  });

  it("refuses a key only a module specifier spells", () => {
    const files = {
      [MAIN]: [
        `import "a.dotted";`,
        `export { x } from "a.reexported";`,
        `export const lazy = () => import("a.dynamic");`,
      ].join("\n"),
    };
    const keys = ["a.dotted", "a.reexported", "a.dynamic"];
    expect(verdict(files, keys)).toEqual(keys);
  });

  it("lets a template vouch only for the keys its types allow", () => {
    const files = {
      [MAIN]: [
        `type Kind = "open" | "won";`,
        `declare const kind: Kind;`,
        `declare const n: 1 | 2;`,
        `t(\`a.status.\${kind}\`);`,
        `t(\`a.\${kind}.title\`);`,
        `t(\`a.step\${n}\`);`,
      ].join("\n"),
    };
    const keys = [
      "a.status.open",
      "a.status.won",
      "a.status.lost",
      "a.won.title",
      "a.won.sub",
      "a.step1",
      "a.step3",
    ];
    expect(verdict(files, keys)).toEqual([
      "a.status.lost",
      "a.won.sub",
      "a.step3",
    ]);
  });

  it("narrows a template's type by the branch it sits in", () => {
    const files = {
      [MAIN]: [
        `type Status = "set" | "missing" | "withheld";`,
        `export const text = (status: Status) => {`,
        `  if (status === "set") return "";`,
        `  return t(\`a.goal.\${status}\`);`,
        `};`,
      ].join("\n"),
    };
    const keys = ["a.goal.set", "a.goal.missing", "a.goal.withheld"];
    expect(verdict(files, keys)).toEqual(["a.goal.set"]);
  });

  it("keeps a plain string substitution vouching for everything under its stem", () => {
    const files = {
      [MAIN]: `declare const name: string; t(\`a.\${name}\`); t(\`b.\${name}.x\`);`,
    };
    const keys = ["a.one", "a.two.deep", "b.one.x", "b.one.y", "c.one"];
    expect(verdict(files, keys)).toEqual(["b.one.y", "c.one"]);
  });

  it("reads a plural base built from a template by its types", () => {
    const files = {
      [MAIN]: `declare const k: "x"; plural(\`a.count.\${k}\`, 2);`,
    };
    const keys = [
      "a.count.x_one",
      "a.count.x_other",
      "a.count.y_one",
      "a.count.y_other",
    ];
    expect(verdict(files, keys)).toEqual(["a.count.y_one", "a.count.y_other"]);
  });

  it("does not let a dot-headed template outside t() vouch past its types", () => {
    const files = {
      [MAIN]: `declare const ext: "png"; export const file = \`logo.\${ext}\`;`,
    };
    expect(verdict(files, ["logo.png", "logo.svg"])).toEqual(["logo.svg"]);
  });

  it("reports a loaded module the checked program does not hold", () => {
    const files = {
      [MAIN]: `import "./card";`,
      "/app/src/card.tsx": `t("a.card");`,
    };
    const graph = moduleGraph({
      tree: new Map(Object.entries(files)),
      aliases: new Map(),
      catalogs: new Set(),
    });
    const { [MAIN]: main } = files;
    const use = liveKeyUse(
      programOver({ [MAIN]: main }),
      graph.reach([MAIN]),
      [MAIN],
      new Set(),
    );
    expect(use.missing).toEqual(["/app/src/card.tsx"]);
  });

  it("counts a class, enum or namespace that runs code when it loads", () => {
    const files = {
      [MAIN]: `import "./defs";`,
      "/app/src/defs.ts": [
        `declare function deco(x: unknown): any;`,
        `declare function mixin(x: unknown): any;`,
        `export class Decorated { @deco(t("a.decorator")) x = 1; }`,
        `export class Extended extends mixin(t("a.extends")) {}`,
        `export class Computed { [t("a.computed")] = 1; }`,
        `export class Static { static label = t("a.static"); }`,
        `export class Block { static { t("a.block"); } }`,
        `export enum Mode { A = t("a.enum") }`,
        `export namespace Space { t("a.namespace"); }`,
        `export class Quiet { label() { return t("a.quiet"); } }`,
      ].join("\n"),
    };
    const keys = [
      "a.decorator",
      "a.extends",
      "a.computed",
      "a.static",
      "a.block",
      "a.enum",
      "a.namespace",
      "a.quiet",
    ];
    expect(verdict(files, keys)).toEqual(["a.quiet"]);
  });
});

describe("the orphan-keys rule", () => {
  // The real entry names every English key but one; a story names that one.
  const [kept, ...named] = Object.keys(en).sort(stable);

  it("reports the one key only a story renders", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "bootstrap.ts"),
        area: "src",
        text: `export const KEYS = ${JSON.stringify(named)};`,
      },
      {
        path: join(srcDir, "bootstrap.stories.tsx"),
        area: "src",
        text: `export const KEY = ${JSON.stringify(kept)};`,
      },
    ]);
    expect(findings).toEqual([kept]);
  });
});
