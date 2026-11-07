// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";

const FRONTEND = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export const MATERIAL_SYMBOLS_LICENSE_SOURCE = resolve(
  FRONTEND,
  "src/design-system/material-symbols/LICENSE",
);

/** Apache-2.0 asks that the notice travel with the copied marks, so the build emits it beside them. */
export function materialSymbolsLicense(): Plugin {
  return {
    name: "margince-material-symbols-license",
    apply: "build",
    generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: "licenses/material-symbols.txt",
        source: readFileSync(MATERIAL_SYMBOLS_LICENSE_SOURCE, "utf8"),
      });
    },
  };
}
