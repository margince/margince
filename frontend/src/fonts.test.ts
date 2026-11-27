// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { get } from "node:http";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { expect, it } from "vitest";
import spaConfig from "../vite.config";
import { stable } from "./format/collate";

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
  const byFile = (a: Face, b: Face) => stable(a.file, b.file);
  expect([...own].sort(byFile)).toEqual(packaged.sort(byFile));
});

// A heading in Vietnamese must find every Vietnamese letter in its own face.
it("sets Vietnamese headings in a face that loads the Vietnamese subset", () => {
  const tokens = readFileSync(
    join(frontendRoot, "src/design-system/tokens.css"),
    "utf8",
  );
  const rule = /:root:lang\(vi\)\s*\{([^}]*)\}/.exec(tokens)?.[1] ?? "";
  const family = /--fontFamilyHeading:\s*"([^"]+)"/.exec(rule)?.[1];
  const vietnameseWeights = own
    .filter(
      (face) => face.family === family && face.file.includes("-vietnamese-"),
    )
    .map((face) => face.weight);
  expect(vietnameseWeights.sort(stable)).toEqual(["400", "500", "700"]);
});

it("swaps every face in when it arrives", () => {
  const displays = [...declared.matchAll(/@font-face\s*\{([^}]*)\}/g)].map(
    ([, block]) => descriptor(block, "font-display"),
  );
  expect(displays).not.toHaveLength(0);
  expect(new Set(displays)).toEqual(new Set(["swap"]));
});

// The suite refuses `fetch`, so the dev server is asked over node:http.
function served(url: string): Promise<{ status: number; body: string }> {
  return new Promise((resolve, reject) => {
    get(url, (response) => {
      let body = "";
      response.setEncoding("latin1");
      response.on("data", (part: string) => {
        body += part;
      });
      response.on("end", () =>
        resolve({ status: response.statusCode ?? 0, body }),
      );
    }).on("error", reject);
  });
}

// Vite names a packaged face by its real path in the pnpm store, which the dev
// server refuses unless fs.allow lists it.
it("serves every face to the dev server", async () => {
  const server = await createServer({
    ...spaConfig,
    configFile: false,
    root: frontendRoot,
    logLevel: "silent",
    optimizeDeps: { noDiscovery: true },
    server: { ...spaConfig.server, port: 0, proxy: undefined },
  });
  try {
    await server.listen();
    const origin = server.resolvedUrls?.local[0];
    if (!origin) throw new Error("the dev server reported no address");
    const sheet = await served(`${origin}src/fonts.css?direct`);
    expect(sheet.status).toBe(200);
    const urls = [...sheet.body.matchAll(/url\("?([^")]+)"?\)/g)].map(
      (match) => new URL(match[1], origin).href,
    );
    expect(urls).toHaveLength(own.length);
    const refused = [];
    for (const url of urls) {
      const { status } = await served(url);
      if (status !== 200) refused.push(`${status} ${url}`);
    }
    expect(refused).toEqual([]);
  } finally {
    await server.close();
  }
}, 60_000);
