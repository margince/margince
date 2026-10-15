// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "vitest";
import { mostSpecificFirst } from "./recordlinks";

test("the most specific record leads, as the Worklist names it", () => {
  const filed = mostSpecificFirst([
    { entity_type: "company", entity_id: "co-1" },
    { entity_type: "contact", entity_id: "ct-1" },
    { entity_type: "project", entity_id: "pr-1" },
    { entity_type: "deal", entity_id: "de-1" },
    { entity_type: "lead", entity_id: "le-1" },
  ]);
  expect(filed.map((link) => link.entity_type)).toEqual([
    "lead",
    "deal",
    "project",
    "contact",
    "company",
  ]);
});

test("two records of one kind keep the order they arrived in", () => {
  const filed = mostSpecificFirst([
    { entity_type: "contact", entity_id: "ct-2" },
    { entity_type: "deal", entity_id: "de-1" },
    { entity_type: "contact", entity_id: "ct-1" },
  ]);
  expect(filed.map((link) => link.entity_id)).toEqual(["de-1", "ct-2", "ct-1"]);
});
