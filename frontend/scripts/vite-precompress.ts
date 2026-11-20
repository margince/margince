// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdir, readFile, writeFile } from "node:fs/promises";
import { extname, join, resolve } from "node:path";
import { promisify } from "node:util";
import { constants, gzip } from "node:zlib";
import type { Plugin } from "vite";

const compress = promisify(gzip);

/** What the build writes a `.gz` beside, by extension, with the type nginx answers it under. */
export const PRECOMPRESSED_TYPES: Readonly<Record<string, string>> = {
  ".js": "application/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".html": "text/html",
  ".webmanifest": "application/manifest+json",
  ".txt": "text/plain",
};

// nginx.conf's gzip_min_length: below it a response goes out as it is.
export const MIN_BYTES = 1024;

export async function precompressedFiles(
  dir: string,
): Promise<{ path: string; bytes: Buffer }[]> {
  const entries = await readdir(dir, { recursive: true, withFileTypes: true });
  const written = await Promise.all(
    entries
      .filter(
        (entry) =>
          entry.isFile() &&
          Object.hasOwn(PRECOMPRESSED_TYPES, extname(entry.name)),
      )
      .map(async (entry) => {
        const path = join(entry.parentPath, entry.name);
        const source = await readFile(path);
        if (source.length < MIN_BYTES) return [];
        const bytes = await compress(source, {
          level: constants.Z_BEST_COMPRESSION,
        });
        return bytes.length < source.length
          ? [{ path: `${path}.gz`, bytes }]
          : [];
      }),
  );
  return written.flat();
}

/** For nginx's `gzip_static`; after writeBundle, both the bundle and public/ are on disk. */
export function precompress(): Plugin {
  let outDir = "";
  return {
    name: "precompress",
    apply: "build",
    configResolved(config) {
      outDir = resolve(config.root, config.build.outDir);
    },
    writeBundle: {
      order: "post",
      sequential: true,
      async handler() {
        const files = await precompressedFiles(outDir);
        await Promise.all(
          files.map(({ path, bytes }) => writeFile(path, bytes)),
        );
      },
    },
  };
}
