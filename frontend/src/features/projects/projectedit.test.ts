// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";
import { stable } from "../../format/collate";

const HERE = fileURLToPath(new URL(".", import.meta.url));
const PROJECT_PATCH = /\bPATCH\(\s*"\/projects\/\{id\}"/;

// The details sections and "Edit project" save through saveRecordEdit, so a
// screen here that PATCHes the project itself is a second writer.
it("leaves the owner pick as the folder's only direct project PATCH", () => {
  const writers = readdirSync(HERE)
    .filter((name) => /\.tsx?$/.test(name) && !/\.(test|stories)\./.test(name))
    .filter((name) => PROJECT_PATCH.test(readFileSync(HERE + name, "utf8")))
    .sort(stable);
  expect(writers).toEqual(["projectowner.tsx"]);
});
