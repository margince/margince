// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { build } from "vite";
import { afterEach, beforeEach, expect, it } from "vitest";
import { SCREEN_CHUNKS_ID } from "../src/lib/screenchunks";
import { screenChunks, screenCodeSpecifiers } from "./vite-screen-chunks";

let dir = "";

beforeEach(async () => {
  dir = await realpath(await mkdtemp(join(tmpdir(), "screen-chunks-")));
});

afterEach(async () => {
  await rm(dir, { recursive: true, force: true });
});

async function shell(screenCode: string, files: Record<string, string>) {
  await Promise.all(
    Object.entries({
      "index.html": '<script type="module" src="/screencode.js"></script>',
      "screencode.js": screenCode,
      ...files,
    }).map(([name, text]) => writeFile(join(dir, name), text)),
  );
  const result = await build({
    root: dir,
    configFile: false,
    base: "/crm/",
    logLevel: "silent",
    plugins: [screenChunks(join(dir, "screencode.js"))],
    build: { write: false },
  });
  const outputs = (Array.isArray(result) ? result : [result]).flatMap(
    (output) => ("output" in output ? output.output : []),
  );
  const html = outputs.find((file) => file.fileName === "index.html");
  if (html?.type !== "asset") throw new Error("the fixture built no shell");
  return { markup: String(html.source), outputs };
}

it("writes each screen's chunk, its static imports and their CSS into the shell as data", async () => {
  const { markup, outputs } = await shell(
    'export const SCREEN_CODE = { deals: () => import("./deals.js"), leads: () => import("./leads.js") }; globalThis.screens = SCREEN_CODE;',
    {
      "deals.js":
        'import { shared } from "./shared.js"; import "./deals.css"; export const deals = shared;',
      "deals.css": ".deals { color: red; }",
      "leads.js":
        'import { shared } from "./shared.js"; export const leads = shared + 1;',
      "shared.js": "export const shared = globalThis.seed ?? 1;",
    },
  );
  const block = new RegExp(
    `<script type="application/json" id="${SCREEN_CHUNKS_ID}">([^<]*)</script>`,
  ).exec(markup);
  expect(block, "the shell carries the map as a data block").not.toBeNull();
  const map: { files: string[]; screens: Record<string, number[]> } =
    JSON.parse(block?.[1] ?? "{}");
  const fileOf = (module: string) =>
    outputs.find(
      (file) =>
        file.type === "chunk" && Object.hasOwn(file.modules, join(dir, module)),
    )?.fileName;
  const files = (screen: string) =>
    (map.screens[screen] ?? []).map((index) => map.files[index]);
  expect(Object.keys(map.screens)).toEqual(["deals", "leads"]);
  const sheet = outputs.find(
    (file) => file.type === "asset" && file.fileName.endsWith(".css"),
  )?.fileName;
  expect(files("deals")).toEqual([
    `/crm/${fileOf("shared.js")}`,
    `/crm/${fileOf("deals.js")}`,
    `/crm/${sheet}`,
  ]);
  expect(files("leads")).toEqual([
    `/crm/${fileOf("shared.js")}`,
    `/crm/${fileOf("leads.js")}`,
  ]);
});

it("fails the build on a screen whose code is not one import()", async () => {
  await expect(
    shell(
      'export const SCREEN_CODE = { deals: () => import("./deals.js"), gone: () => Promise.resolve() };',
      { "deals.js": "export const deals = 1;" },
    ),
  ).rejects.toThrow('SCREEN_CODE.gone is not `() => import("…")`');
});

it("fails the build when a screen's module does not resolve", async () => {
  await expect(
    shell(
      'export const SCREEN_CODE = { gone: () => import("./gone.js") };',
      {},
    ),
  ).rejects.toThrow(/gone\.js/);
});

it("refuses a file with no SCREEN_CODE to read", async () => {
  await expect(
    screenCodeSpecifiers("export const OTHER = {};"),
  ).rejects.toThrow("no SCREEN_CODE entry was read");
});
