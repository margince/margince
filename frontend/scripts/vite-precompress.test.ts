// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { randomBytes } from "node:crypto";
import { existsSync } from "node:fs";
import {
  mkdir,
  mkdtemp,
  readdir,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { constants, gunzipSync, gzipSync } from "node:zlib";
import { build } from "vite";
import { expect, it } from "vitest";
import { stable } from "../src/format/collate";
import { MIN_BYTES, precompress } from "./vite-precompress";

const words = (count: number) =>
  Array.from({ length: count }, (_, index) => `word${index % 50}`).join(" ");

it("writes a level-9 .gz beside every text file of the build worth compressing", async () => {
  const dir = await realpath(await mkdtemp(join(tmpdir(), "precompress-")));
  try {
    await mkdir(join(dir, "public"));
    await writeFile(
      join(dir, "index.html"),
      `<!doctype html><title>${words(300)}</title><script type="module" src="./entry.js"></script>`,
    );
    await writeFile(
      join(dir, "entry.js"),
      `import "./entry.css"; console.log(${JSON.stringify(words(400))});`,
    );
    await writeFile(
      join(dir, "entry.css"),
      `body::after { content: ${JSON.stringify(words(400))}; }`,
    );
    await writeFile(join(dir, "public", "robots.txt"), words(300));
    await writeFile(
      join(dir, "public", "short.txt"),
      "x".repeat(MIN_BYTES - 1),
    );
    await writeFile(join(dir, "public", "noise.txt"), randomBytes(4096));
    await writeFile(join(dir, "public", "photo.png"), words(300));
    await build({
      root: dir,
      configFile: false,
      logLevel: "silent",
      plugins: [precompress()],
      build: { outDir: join(dir, "dist") },
    });
    const dist = join(dir, "dist");
    const files = (await readdir(dist, { recursive: true })).map(String);
    const compressed = files
      .filter((name) => name.endsWith(".gz"))
      .sort(stable);
    const script = files.find((name) => /^assets\/index-.*\.js$/.test(name));
    const sheet = files.find((name) => /^assets\/index-.*\.css$/.test(name));
    expect(compressed).toEqual(
      [`${script}.gz`, `${sheet}.gz`, "index.html.gz", "robots.txt.gz"].sort(
        stable,
      ),
    );
    for (const name of compressed) {
      const source = await readFile(join(dist, name.replace(/\.gz$/, "")));
      const gz = await readFile(join(dist, name));
      expect(gunzipSync(gz).equals(source), name).toBe(true);
      expect(gz.length, name).toBe(
        gzipSync(source, { level: constants.Z_BEST_COMPRESSION }).length,
      );
    }
    // Built, and left as they are: too short, already dense, or not text.
    for (const name of ["short.txt", "noise.txt", "photo.png"]) {
      expect(existsSync(join(dist, name)), name).toBe(true);
    }
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

it("writes nothing for a build with no app shell, as Storybook's", async () => {
  const dir = await realpath(await mkdtemp(join(tmpdir(), "precompress-")));
  try {
    await writeFile(
      join(dir, "iframe.html"),
      `<!doctype html><title>${words(300)}</title><script type="module" src="./story.js"></script>`,
    );
    await writeFile(
      join(dir, "story.js"),
      `console.log(${JSON.stringify(words(400))});`,
    );
    await build({
      root: dir,
      configFile: false,
      logLevel: "silent",
      plugins: [precompress()],
      build: {
        outDir: join(dir, "dist"),
        rolldownOptions: { input: join(dir, "iframe.html") },
      },
    });
    const files = (await readdir(join(dir, "dist"), { recursive: true })).map(
      String,
    );
    expect(files).toContain("iframe.html");
    expect(files.filter((name) => name.endsWith(".gz"))).toEqual([]);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
