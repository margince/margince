// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { extensionLayers, filesMatching } from "./source-tree";

export const srcDir = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "src",
);

const extensionsDir = join(srcDir, "..", "..", "extensions");

const underTree = (pattern: RegExp) =>
  filesMatching(srcDir, pattern).concat(
    extensionLayers(extensionsDir).flatMap((l) => filesMatching(l, pattern)),
  );

export const sheetTexts = () =>
  underTree(/\.css$/).map((f) => readFileSync(f, "utf8"));

// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
export const markupFiles = () =>
  underTree(/\.(tsx|jsx)$/).filter((f) => !/\.test\.(tsx|jsx)$/.test(f));
