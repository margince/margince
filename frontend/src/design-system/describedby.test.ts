// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { describedByIds } from "./describedby";

it("joins the ids in the order given", () => {
  expect(describedByIds("hint", "reason", "refusal")).toBe(
    "hint reason refusal",
  );
});

it("drops an absent id, whatever spells its absence", () => {
  expect(describedByIds(undefined, "hint", null, false, "", "refusal")).toBe(
    "hint refusal",
  );
});

it("names nothing when no id is left", () => {
  expect(describedByIds()).toBeUndefined();
  expect(describedByIds(undefined, false, null, "")).toBeUndefined();
});
