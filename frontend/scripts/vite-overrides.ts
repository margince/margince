// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync, realpathSync } from "node:fs";
import { join, resolve } from "node:path";
import type { Plugin } from "vite";

// One core frontend file a unit replaces, as gen-composition wrote it to
// build/composition-frontend/overrides.json after checking the unit's lock.
export type FileOverride = {
  unit: string;
  target: string;
  source: string;
};

const CORE_ORIGINAL = "@core-original/";

// readFileOverrides is empty unless a composed lane asks, so the vanilla bundle
// stays core's own.
export function readFileOverrides(
  file: string,
  composed: boolean,
): FileOverride[] {
  return composed && existsSync(file)
    ? (JSON.parse(readFileSync(file, "utf8")) as FileOverride[])
    : [];
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
  return {
    name: "margince-core-overrides",
    enforce: "pre",
    async resolveId(source, importer, options) {
      if (source.startsWith(CORE_ORIGINAL)) {
        return this.resolve(
          join(frontendRoot, source.slice(CORE_ORIGINAL.length)),
          importer,
          { ...options, skipSelf: true },
        );
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
