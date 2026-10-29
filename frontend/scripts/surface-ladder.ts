// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Flat CSS cannot count with a list named once, so each depth carries a copy.
// Usage: node scripts/surface-ladder.ts [surface…] [-- overlay…]

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { ladderLists, ladderSheet } from "./lib/surface-ladder.ts";

const sheet = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "src",
  "design-system",
  "surfaces.css",
);

const current = ladderLists(readFileSync(sheet, "utf8"));
const added = process.argv.slice(2);
const split = added.indexOf("--");
const surfaces = [
  ...current.surfaces,
  ...(split < 0 ? added : added.slice(0, split)),
];
const overlays = [
  ...current.overlays,
  ...(split < 0 ? [] : added.slice(split + 1)),
];
writeFileSync(sheet, ladderSheet(surfaces, overlays));
