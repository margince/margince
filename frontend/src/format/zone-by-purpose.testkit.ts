// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const srcRoot = join(here, "..");

// The one home for the rule. It names both zones because that is its job.
export const zoneModule = join(here, "timezone.ts");

// Every source file under `dir` that a zone gate reads. `gate` is the test
// calling this: it spells out the patterns it hunts for, and the module is the
// permitted site. A sweep that read either would vouch for itself.
export function sourceFiles(dir: string, gate: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sourceFiles(path, gate);
    }
    if (path === gate || path === zoneModule) {
      return [];
    }
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}
