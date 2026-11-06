import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, it } from "vitest";
import { essentialKeys } from "./essential-copy";

const directories: string[] = [];
function fixture(bootstrap: string) {
  const root = mkdtempSync(join(tmpdir(), "essential-copy-"));
  directories.push(root);
  mkdirSync(join(root, "src"));
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
