// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Flat CSS cannot count with a list named once, so each depth carries a copy.
// Usage: node scripts/surface-ladder.ts [surface…] [-- overlay…]
//        node scripts/surface-ladder.ts --remove selector…

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { editedLists, ladderLists, ladderSheet } from "./lib/surface-ladder.ts";

const sheet = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "src",
  "design-system",
  "surfaces.css",
);

const lists = editedLists(
  ladderLists(readFileSync(sheet, "utf8")),
  process.argv.slice(2),
);
writeFileSync(sheet, ladderSheet(lists.surfaces, lists.overlays));
