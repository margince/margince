// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import type { Translator } from "../i18n";
import { en } from "../i18n/en";
import { omittedFactorsText } from "./brief.factors";

const t: Translator = (key, params) =>
  en[key].replace(/\{(\w+)\}/g, (whole, name: string) =>
    params && name in params ? String(params[name]) : whole,
  );

it("says nothing when the run weighed every factor", () => {
  expect(omittedFactorsText([], t)).toBeNull();
});

it("names the withheld factor and says the order does not account for it", () => {
  const text = omittedFactorsText(["warmth"], t);
  expect(text).toContain(en["brief.factor.warmth"]);
  expect(text).toContain("does not account for");
});

it("renders a factor this build cannot name rather than dropping it", () => {
  const text = omittedFactorsText(["revenue"], t);
  expect(text).not.toBeNull();
  expect(text).toContain(en["brief.factor.unknown"]);
});

it("folds several unknown factors into one phrase, not a repeated one", () => {
  const text = omittedFactorsText(["revenue", "timing"], t) ?? "";
  expect(text.split(en["brief.factor.unknown"])).toHaveLength(2);
});

it("keeps a named factor beside an unknown one", () => {
  const text = omittedFactorsText(["warmth", "revenue"], t) ?? "";
  expect(text).toContain(en["brief.factor.warmth"]);
  expect(text).toContain(en["brief.factor.unknown"]);
});
