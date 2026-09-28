// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { type Translator, translate } from "../i18n";
import { en } from "../i18n/en";
import { omittedFactorsText } from "./brief.factors";

const t: Translator = (key, params) => translate("en", key, params);

it("says nothing when the run weighed every factor", () => {
  expect(omittedFactorsText([], t, "en")).toBeNull();
});

it("names the withheld factor", () => {
  expect(omittedFactorsText(["warmth"], t, "en")).toContain(
    en["brief.factor.warmth"],
  );
});

// The caveat states WHAT the order lost and never why. `warmth` is missing
// because a grant withheld it; any other token is one this build knows nothing
// about, so a frame naming a cause would guess, on a line that offers no retry.
it("claims no reason for the omission", () => {
  const text = omittedFactorsText(["warmth"], t, "en") ?? "";
  expect(text).not.toMatch(/role|permission|hidden/i);
});

it("renders a factor this build cannot name rather than dropping it", () => {
  const text = omittedFactorsText(["revenue"], t, "en");
  expect(text).not.toBeNull();
  expect(text).toContain(en["brief.factor.unknown"]);
});

it("folds several unknown factors into one phrase, not a repeated one", () => {
  const text = omittedFactorsText(["revenue", "timing"], t, "en") ?? "";
  expect(text.split(en["brief.factor.unknown"])).toHaveLength(2);
});

// The wire field carries no `uniqueItems`, so a repeat is the server's to send
// and this function's to absorb.
it("names a repeated factor once", () => {
  const text = omittedFactorsText(["warmth", "warmth"], t, "en") ?? "";
  expect(text.split(en["brief.factor.warmth"])).toHaveLength(2);
});

// A token naming something every object inherits. Looked up in an object
// literal these resolve to functions rather than to a miss, which is a crash
// one version skew away: the server names the factor and the translator is
// handed `Object.prototype.toString`.
it.each(["toString", "constructor", "hasOwnProperty", "__proto__"])(
  "treats the inherited name %s as a factor it cannot name",
  (factor) => {
    expect(omittedFactorsText([factor], t, "en")).toContain(
      en["brief.factor.unknown"],
    );
  },
);

it("joins two factors with the reader's own conjunction", () => {
  const text = omittedFactorsText(["warmth", "revenue"], t, "en") ?? "";
  expect(text).toContain(en["brief.factor.warmth"]);
  expect(text).toContain(en["brief.factor.unknown"]);
  expect(text).toContain(" and ");
});

it("joins them in German without borrowing the English conjunction", () => {
  const de: Translator = (key, params) => translate("de", key, params);
  const text = omittedFactorsText(["warmth", "revenue"], de, "de") ?? "";
  expect(text).toContain(" und ");
  expect(text).not.toContain(" and ");
});
