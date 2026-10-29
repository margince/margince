// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync, realpathSync } from "node:fs";
import { join, resolve } from "node:path";
import type { Plugin } from "vite";

// One core frontend file a unit replaces, as gen-composition wrote it to
// build/composition-frontend/overrides.json.
export type FileOverride = {
  unit: string;
  target: string;
  source: string;
};

const CORE_ORIGINAL = "@core-original/";

const stripExtension = (path: string) => path.replace(/\.[cm]?[jt]sx?$/, "");

// readFileOverrides is empty unless a composed lane asks, so the vanilla bundle
// stays core's own. gen-composition always writes the file, so a composed lane
// without it would ship core's files in place of every replacement.
export function readFileOverrides(
  file: string,
  composed: boolean,
): FileOverride[] {
  if (!composed) {
    return [];
  }
  if (!existsSync(file)) {
    throw new Error(
      `${file} is missing — run 'make -C backend composition' before building the composed frontend lane`,
    );
  }
  return JSON.parse(readFileSync(file, "utf8")) as FileOverride[];
}

// overrideCoreFiles resolves every import of a replaced core file to the unit's
// file, whatever specifier reached it. "@core-original/<path>" always means
// core's own file, so a replacement can wrap the component it replaces.
export function overrideCoreFiles(
  frontendRoot: string,
  overrides: readonly FileOverride[],
): Plugin {
  // Keyed by real path, because that is the id Vite resolves a module to.
  const byTarget = new Map(
    overrides.map((o) => [
      realpathSync(resolve(frontendRoot, o.target)),
      o.source,
    ]),
  );
  // A replacement may reach core's copy of the one file it replaces, and nothing
  // else; ext-imports refuses the rest before a build gets here.
  const originalOf = new Map(
    overrides.map((o) => [
      realpathSync(o.source),
      resolve(frontendRoot, o.target),
    ]),
  );
  const coreOriginal = (source: string, importer: string | undefined) => {
    const original =
      importer && existsSync(importer)
        ? originalOf.get(realpathSync(importer))
        : undefined;
    const wanted = join(frontendRoot, source.slice(CORE_ORIGINAL.length));
    if (!original || stripExtension(original) !== stripExtension(wanted)) {
      throw new Error(
        `${importer ?? "an entry"} imports '${source}' — only a replacement may, and only core's copy of the file it replaces`,
      );
    }
    return original;
  };
  return {
    name: "margince-core-overrides",
    enforce: "pre",
    async resolveId(source, importer, options) {
      if (source.startsWith(CORE_ORIGINAL)) {
        return coreOriginal(source, importer);
      }
      if (byTarget.size === 0) {
        return null;
      }
      const resolved = await this.resolve(source, importer, {
        ...options,
        skipSelf: true,
      });
      const replacement = resolved ? byTarget.get(resolved.id) : undefined;
      return replacement && importer !== replacement ? replacement : null;
    },
  };
}
