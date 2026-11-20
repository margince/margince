// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const resolvePackaged = createRequire(
  join(frontendRoot, "package.json"),
).resolve;

type Face = {
  family: string;
  style: string;
  weight: string;
  file: string;
  range: string;
};

function descriptor(block: string, name: string): string {
  const value = new RegExp(`${name}:\\s*([^;]+);`).exec(block)?.[1];
  if (value === undefined) throw new Error(`a @font-face has no ${name}`);
  return value.replace(/\s+/g, " ").trim();
}

function faces(css: string, source: (src: string) => string): Face[] {
  return [...css.matchAll(/@font-face\s*\{([^}]*)\}/g)].map(([, block]) => ({
    family: descriptor(block, "font-family").replace(/^["']|["']$/g, ""),
    style: descriptor(block, "font-style"),
    weight: descriptor(block, "font-weight"),
    file: source(descriptor(block, "src")),
    range: descriptor(block, "unicode-range").replace(/\s/g, ""),
  }));
}

const PACKAGED_FILE = /^@fontsource\/([\w-]+)\/files\/([\w-]+\.woff2)$/;
const declared = readFileSync(join(frontendRoot, "src/fonts.css"), "utf8");
const own = faces(declared, (src) => {
  const url = /^url\("([^"]+)"\) format\("woff2"\)$/.exec(src)?.[1];
  if (url === undefined || !PACKAGED_FILE.test(url))
    throw new Error(`${src} is not one packaged woff2 file`);
  return url;
});

it("declares every subset of each face it loads, as its @fontsource package does", () => {
  const loaded = new Set(
    own.map((face) => `${PACKAGED_FILE.exec(face.file)?.[1]}/${face.weight}`),
  );
  const packaged = [...loaded].flatMap((sheet) =>
    faces(
      readFileSync(resolvePackaged(`@fontsource/${sheet}.css`), "utf8"),
      (src) => {
        const file = /url\(\.\/files\/([\w-]+\.woff2)\)/.exec(src)?.[1];
        if (file === undefined) throw new Error(`${sheet} names no woff2`);
        return `@fontsource/${sheet.split("/")[0]}/files/${file}`;
      },
    ),
  );
  const byFile = (a: Face, b: Face) => a.file.localeCompare(b.file);
  expect([...own].sort(byFile)).toEqual(packaged.sort(byFile));
});

it("swaps every face in when it arrives", () => {
  const displays = [...declared.matchAll(/@font-face\s*\{([^}]*)\}/g)].map(
    ([, block]) => descriptor(block, "font-display"),
  );
  expect(displays).not.toHaveLength(0);
  expect(new Set(displays)).toEqual(new Set(["swap"]));
});
