import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, expect, it } from "vitest";
import { initialEntries } from "../vite-entry-chunks";
import { essentialKeys } from "./essential-copy";

const directories: string[] = [];
function fixture(bootstrap: string) {
  const root = mkdtempSync(join(tmpdir(), "essential-copy-"));
  directories.push(root);
  for (const entry of Object.values(initialEntries(root)).flat()) {
    mkdirSync(dirname(entry), { recursive: true });
    writeFileSync(entry, "export {};");
  }
  for (const [name, source] of Object.entries({
    "bootstrap.ts": bootstrap,
    "main.tsx": 'export const title = "common.loading";',
    "App.analytics.tsx": 'export const label = "common.close";',
    "deferred.ts": 'export const label = "common.undo";',
  }))
    writeFileSync(join(root, "src", name), source);
  return root;
}
afterEach(() => {
  for (const dir of directories.splice(0)) rmSync(dir, { recursive: true });
});

it("selects static entry copy without vouching for unopened modules", () => {
  const keys = essentialKeys(fixture('void import("./deferred");'));
  expect([...keys].sort()).toEqual(["common.close", "common.loading"]);
});

it("refuses a missing entry instead of generating an incomplete catalog", () => {
  const root = fixture('export const label = "common.loading";');
  rmSync(join(root, "src/App.analytics.tsx"));
  expect(() => essentialKeys(root)).toThrow("Missing initial entry");
});

it.each([
  'import { lazy } from "react"; export const View = lazy(() => import("./deferred"));',
  'import { lazy as later } from "react"; export const View = later(() => import("./deferred"));',
  'import * as R from "react"; export const View = R.lazy(() => import("./deferred"));',
  'import R from "react"; export const View = R["lazy"](() => import("./deferred"));',
])("refuses a lazy boundary that skips complete copy: %s", (source) => {
  expect(() => essentialKeys(fixture(source))).toThrow("must use i18n/lazy");
});

it.each([
  'import { lazy } from "./i18n/lazy";',
  'import { lazy as later } from "./i18n/lazy";',
  'import * as words from "./i18n/lazy";',
])("derives essential deferred copy from its boundary: %s", (declaration) => {
  const callee = declaration.includes("later")
    ? "later"
    : declaration.includes("words")
      ? "words.lazy"
      : "lazy";
  const root = fixture(
    `${declaration} export const View = ${callee}(() => import("./deferred"), "essential");`,
  );
  mkdirSync(join(root, "src/i18n"));
  writeFileSync(
    join(root, "src/i18n/lazy.tsx"),
    "export const lazy = () => null;",
  );
  expect(essentialKeys(root).has("common.undo")).toBe(true);
});

it("refuses an essential deferred import it cannot resolve", () => {
  const root = fixture(
    'import { lazy } from "./i18n/lazy"; export const View = lazy(() => import("./missing"), "essential");',
  );
  mkdirSync(join(root, "src/i18n"));
  writeFileSync(
    join(root, "src/i18n/lazy.tsx"),
    "export const lazy = () => null;",
  );
  expect(() => essentialKeys(root)).toThrow(
    "Essential lazy import cannot be resolved",
  );
});

it.each([
  ['"complete"', false],
  ["globalThis.level", true],
] as const)(
  "keeps the deferred catalog census complete for level %s",
  (level, included) => {
    const root = fixture(
      `import { lazy } from "./i18n/lazy"; export const View = lazy(() => import("./deferred"), ${level});`,
    );
    mkdirSync(join(root, "src/i18n"));
    writeFileSync(
      join(root, "src/i18n/lazy.tsx"),
      "export const lazy = () => null;",
    );
    expect(essentialKeys(root).has("common.undo")).toBe(included);
  },
);

it("reaches nested essential boundaries", () => {
  const root = fixture(
    'import { lazy } from "./i18n/lazy"; export const View = lazy(() => import("./deferred"), "essential");',
  );
  mkdirSync(join(root, "src/i18n"));
  writeFileSync(
    join(root, "src/i18n/lazy.tsx"),
    "export const lazy = () => null;",
  );
  writeFileSync(
    join(root, "src/deferred.ts"),
    'import { lazy } from "./i18n/lazy"; export const View = lazy(() => import("./nested"), "essential");',
  );
  writeFileSync(
    join(root, "src/nested.ts"),
    'export const label = "common.undo";',
  );
  expect(essentialKeys(root).has("common.undo")).toBe(true);
});
