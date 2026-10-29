// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  appStylesheets,
  declaredValues,
  rulesOf,
  subjectClasses,
  subjectsOf,
} from "../../scripts/lib/css-rules";

// The sidebar is glass over the lit page, never a dark green column and never
// the accent (the shell in DESIGN.md). The dark plate (`--bgInkField`) is for
// what passes over the page, and is easy to mistake for the sidebar's.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

// The panel itself in any state or layout: a rule whose subject is `.rail`,
// not a row or tooltip inside it.
const panelRules = rulesOf(appStylesheets(frontendRoot)).filter(
  (rule) =>
    subjectsOf(rule.selector).length === 1 &&
    subjectClasses(rule.selector).has("rail") &&
    !/\s/.test(rule.selector.trim()),
);

const grounds = panelRules.flatMap((rule) =>
  ["background", "background-color"].flatMap((property) =>
    declaredValues(rule.body, property).map((value) => ({
      selector: rule.selector,
      value,
    })),
  ),
);

describe("the sidebar's ground", () => {
  it("finds the panel's own background", () => {
    expect(grounds.length).toBeGreaterThanOrEqual(1);
  });

  it("is never the dark ink field or the accent", () => {
    const green = grounds.filter(({ value }) =>
      /--(bgInkField|accent)/.test(value),
    );
    expect(green, "the sidebar is glass: `--pane` over the lit page").toEqual(
      [],
    );
  });
});
