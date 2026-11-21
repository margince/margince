// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { build, type Rolldown } from "vite";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { startupStyles } from "./vite-startup-styles";

let dir = "";

beforeAll(async () => {
  dir = await realpath(await mkdtemp(join(tmpdir(), "startup-styles-")));
  const files: Record<string, string | Buffer> = {
    "index.html": '<script type="module" src="/entry.js"></script>',
    "entry.js": 'void import("./main.js"); void import("./side.js");',
    "side.js": 'import "./shared.js";',
    "main.js":
      'import "./shared.js"; import "./main.css"; globalThis.open = () => import("./other.js");',
    "shared.js": 'import "./shared.css"; globalThis.shared = 1;',
    "other.js": 'import "./shared.js"; import "./other.css";',
    "shared.css": ".shared { color: red; }",
    "main.css":
      '@font-face { font-family: Face; src: url("./face.woff2") format("woff2"); } .main { font-family: Face; }',
    "other.css":
      '@font-face { font-family: Late; src: url("./late.woff2") format("woff2"); } .other { font-family: Late; }',
    "face.woff2": Buffer.alloc(8192, 1),
    "late.woff2": Buffer.alloc(8192, 2),
  };
  await Promise.all(
    Object.entries(files).map(([name, text]) =>
      writeFile(join(dir, name), text),
    ),
  );
});

afterAll(async () => {
  await rm(dir, { recursive: true, force: true });
});

async function bundle(fonts: readonly string[]) {
  const result = await build({
    root: dir,
    configFile: false,
    base: "/crm/",
    logLevel: "silent",
    plugins: [startupStyles([join(dir, "main.js")], fonts)],
    build: { write: false },
  });
  const outputs = (Array.isArray(result) ? result : [result]).flatMap(
    (output) => ("output" in output ? output.output : []),
  );
  const html = outputs.find((file) => file.fileName === "index.html");
  if (html?.type !== "asset") throw new Error("the fixture built no shell");
  return { markup: String(html.source), outputs };
}

const named = (
  outputs: (Rolldown.OutputChunk | Rolldown.OutputAsset)[],
  name: string,
) =>
  outputs.find((file) => file.type === "asset" && file.names.includes(name))
    ?.fileName;

describe("the startup stylesheets and fonts in the shell", () => {
  it("links the startup chunk's CSS, its imports first, and none a later screen needs", async () => {
    const { markup, outputs } = await bundle([]);
    const sheets = [
      ...markup.matchAll(/<link rel="stylesheet"[^>]*href="([^"]+)"/g),
    ].map((match) => match[1]);
    const shared = outputs.find(
      (file) =>
        file.type === "asset" &&
        file.fileName.endsWith(".css") &&
        String(file.source).includes(".shared"),
    );
    const main = outputs.find(
      (file) =>
        file.type === "asset" &&
        file.fileName.endsWith(".css") &&
        String(file.source).includes(".main"),
    );
    expect(sheets).toEqual([
      `/crm/${shared?.fileName}`,
      `/crm/${main?.fileName}`,
    ]);
    expect(markup).not.toMatch(/other[^"]*\.css/);
  });

  // The runtime loader skips a sheet the document already links by the same URL.
  it("names each sheet by the URL the runtime loader would request", async () => {
    const { markup, outputs } = await bundle([]);
    const entry = outputs.find(
      (file): file is Rolldown.OutputChunk =>
        file.type === "chunk" && file.isEntry,
    );
    for (const [, href] of markup.matchAll(
      /<link rel="stylesheet"[^>]*href="\/crm\/([^"]+)"/g,
    )) {
      expect(entry?.code).toContain(JSON.stringify(href));
    }
  });

  it("preloads a named face the startup stylesheets declare", async () => {
    const { markup, outputs } = await bundle(["face.woff2"]);
    expect(markup).toContain(
      `<link rel="preload" as="font" type="font/woff2" crossorigin="" href="/crm/${named(outputs, "face.woff2")}">`,
    );
    expect(markup).not.toContain(named(outputs, "late.woff2"));
  });

  it("refuses a face only a later screen declares, and one the build never emitted", async () => {
    await expect(bundle(["late.woff2"])).rejects.toThrow(
      "no startup stylesheet declares the preloaded font late.woff2",
    );
    await expect(bundle(["gone.woff2"])).rejects.toThrow(
      "the build emitted no font gone.woff2 to preload",
    );
  });

  it("leaves another page of a build alone, as Storybook's iframe.html", async () => {
    await writeFile(
      join(dir, "iframe.html"),
      '<script type="module" src="/story.js"></script>',
    );
    await writeFile(join(dir, "story.js"), "console.log(1);");
    const result = await build({
      root: dir,
      configFile: false,
      logLevel: "silent",
      plugins: [startupStyles([join(dir, "main.js")], ["face.woff2"])],
      build: {
        write: false,
        rolldownOptions: { input: join(dir, "iframe.html") },
      },
    });
    const page = (Array.isArray(result) ? result : [result])
      .flatMap((output) => ("output" in output ? output.output : []))
      .find((file) => file.fileName === "iframe.html");
    if (page?.type !== "asset")
      throw new Error("the fixture built no iframe.html");
    expect(String(page.source)).toContain('<script type="module"');
    expect(String(page.source)).not.toMatch(/rel="(?:stylesheet|preload)"/);
  });

  it("refuses an app shell whose startup entries no chunk holds", async () => {
    await expect(
      build({
        root: dir,
        configFile: false,
        logLevel: "silent",
        plugins: [startupStyles([join(dir, "absent.js")], [])],
        build: { write: false },
      }),
    ).rejects.toThrow("no chunk holds the startup entries");
  });
});
