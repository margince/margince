// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { execFileSync } from "node:child_process";
import { mkdtemp, readdir, realpath, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { build } from "vite";
import { expect, it } from "vitest";
import { entryChunks, preloadEntries } from "./vite-entry-chunks";

it("groups static dependencies once and leaves unopened features lazy", async () => {
  const dir = await realpath(await mkdtemp(join(tmpdir(), "entry-chunks-")));
  try {
    const files = {
      "entry.js": 'import "./main.js";',
      "main.js":
        'import { value } from "./shared.js"; console.log(value); globalThis.open = () => import("./analytics.js");',
      "shared.js":
        "export const value = globalThis.seed ?? 42; globalThis.sharedInitializations = (globalThis.sharedInitializations ?? 0) + 1;",
      "analytics.js":
        'import { value } from "./shared.js"; console.log(value); globalThis.settings = () => import("./settings.js");',
      "settings.js": 'console.log("unopened settings");',
    };
    await Promise.all(
      Object.entries(files).map(([name, text]) =>
        writeFile(join(dir, name), text),
      ),
    );
    const startup = entryChunks("startup", [join(dir, "entry.js")]);
    const analytics = entryChunks("analytics", [join(dir, "analytics.js")]);
    const result = await build({
      configFile: false,
      logLevel: "silent",
      plugins: [startup.plugin, analytics.plugin],
      build: {
        write: false,
        rolldownOptions: {
          input: join(dir, "entry.js"),
          output: {
            strictExecutionOrder: true,
            codeSplitting: { groups: [startup.group, analytics.group] },
          },
        },
      },
    });
    const outputs = Array.isArray(result) ? result : [result];
    const chunks = outputs
      .flatMap((output) => ("output" in output ? output.output : []))
      .filter((chunk) => chunk.type === "chunk");
    const startupChunk = chunks.find(
      (chunk) => join(dir, "main.js") in chunk.modules,
    );
    const analyticsChunk = chunks.find(
      (chunk) => join(dir, "analytics.js") in chunk.modules,
    );
    expect(startupChunk).toBeDefined();
    expect(analyticsChunk).toBeDefined();
    expect(analyticsChunk).not.toBe(startupChunk);
    expect(
      chunks.some((chunk) => join(dir, "settings.js") in chunk.modules),
    ).toBe(true);
    expect(startupChunk?.modules).not.toHaveProperty(join(dir, "settings.js"));
    expect(analyticsChunk?.modules).not.toHaveProperty(
      join(dir, "settings.js"),
    );
    expect(
      chunks.filter((chunk) => join(dir, "shared.js") in chunk.modules),
    ).toHaveLength(1);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

it("the production dictionary loader preloads only the selected language", async () => {
  const dir = await realpath(
    await mkdtemp(join(tmpdir(), "dictionary-preloads-")),
  );
  try {
    const source = fileURLToPath(
      new URL("../src/i18n/catalogs.ts", import.meta.url),
    );
    await writeFile(
      join(dir, "entry.js"),
      `import { loadLocale, loadedCatalogs } from ${JSON.stringify(source)}; await loadLocale(globalThis.catalogLocale ?? "en"); export const loaded = [...loadedCatalogs];`,
    );
    await writeFile(join(dir, "package.json"), '{"type":"module"}');
    await build({
      configFile: false,
      logLevel: "silent",
      build: {
        target: "esnext",
        outDir: join(dir, "dist"),
        modulePreload: { polyfill: false },
        rolldownOptions: {
          input: join(dir, "entry.js"),
          preserveEntrySignatures: "strict",
          output: { entryFileNames: "entry.js", strictExecutionOrder: true },
        },
      },
    });
    const assets = await readdir(join(dir, "dist/assets"));
    for (const locale of ["en", "de", "vi"]) {
      expect(assets.some((name) => name.startsWith(`${locale}-`))).toBe(true);
    }
    const happyDom = pathToFileURL(
      createRequire(import.meta.url).resolve("happy-dom"),
    ).href;
    const entry = pathToFileURL(resolve(dir, "dist/entry.js")).href;
    const output = execFileSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        `
      import { Window } from ${JSON.stringify(happyDom)};
      const window = new Window({ url: "http://localhost/" });
      globalThis.document = window.document;
      const { loaded } = await import(${JSON.stringify(entry)});
      const unwanted = [...document.querySelectorAll('link[rel="modulepreload"]')].map(link => link.href).filter(href => /\\/(de|vi)-/.test(href));
      console.log(JSON.stringify({ loaded, unwanted }));
      await window.happyDOM.close();
    `,
      ],
      { encoding: "utf8" },
    );
    const result: unknown = JSON.parse(output.trim());
    expect(result).toEqual({ loaded: ["en"], unwanted: [] });
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

it("preloads the main entry under the configured base without preloading unopened routes", async () => {
  const dir = await realpath(await mkdtemp(join(tmpdir(), "entry-preload-")));
  try {
    await writeFile(
      join(dir, "index.html"),
      '<script type="module" src="/entry.js"></script>',
    );
    await writeFile(join(dir, "entry.js"), 'void import("./main.js");');
    await writeFile(
      join(dir, "main.js"),
      'globalThis.open = () => import("./other.js");',
    );
    await writeFile(join(dir, "other.js"), "globalThis.opened = true;");
    const result = await build({
      root: dir,
      configFile: false,
      base: "/crm/",
      logLevel: "silent",
      plugins: [preloadEntries([join(dir, "main.js")])],
      build: { write: false },
    });
    const outputs = Array.isArray(result) ? result : [result];
    const bundle = outputs.flatMap((output) =>
      "output" in output ? output.output : [],
    );
    const html = bundle.find(
      (file) => file.type === "asset" && file.fileName === "index.html",
    );
    const main = bundle.find(
      (file) =>
        file.type === "chunk" &&
        Object.hasOwn(file.modules, join(dir, "main.js")),
    );
    const other = bundle.find(
      (file) =>
        file.type === "chunk" &&
        Object.hasOwn(file.modules, join(dir, "other.js")),
    );
    if (!html || html.type !== "asset" || !main || !other)
      throw new Error("Missing built fixture entries");
    const markup = String(html.source);
    expect(markup).toContain(`href="/crm/${main.fileName}"`);
    expect(markup).not.toContain(`href="/crm/${other.fileName}"`);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
