// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// When a request omits `is_current_primary`, the server decides it: a
// contact's only current employment becomes the primary one. A screen that
// sends a literal takes that decision away. The rule reads every screen, so a
// screen written tomorrow is in scope.

import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import type { CensusFile, CensusRule } from "../tree";

// A literal only. `is_current_primary: isCurrent ?? undefined` passes on what
// the reader chose, and banning the key would ban saying "yes" too.
export const LITERAL = /is_current_primary:\s*(true|false)\b/;

const SCREEN_DIRS = SCREEN_ROOTS.map((root) => `src/${root}/`);

const isScreen = (file: CensusFile) =>
  file.area === "src" &&
  SCREEN_DIRS.some((dir) => file.rel.startsWith(dir)) &&
  file.path.endsWith(".tsx") &&
  !/\.(test|stories|testkit)\.tsx$/.test(file.path);

export const rule: CensusRule = {
  name: "employmentprimary",
  corpus: (tree) => tree.files.filter(isScreen),
  visit: (file) =>
    LITERAL.test(file.text)
      ? [`${file.rel}: sends is_current_primary as a literal`]
      : [],
  advice:
    "Leave is_current_primary out of the request, or pass what the reader " +
    "chose. The server makes a contact's only current employment the primary one.",
};
