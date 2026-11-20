// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate } from "../../i18n";
import { dealEditFields } from "./deals.fields";

// The deal's probability reads a whole percent as the stage editor does.
const probability = dealEditFields((key) => translate("en", key), {
  partnerOptions: [],
  masked: [],
  closed: false,
  me: "u1",
  currentOwner: null,
  currency: "EUR",
  leadingCurrencies: ["EUR"],
  locale: "en",
}).find((field) => field.key === "win_probability");

it.each(["50.0", "1e1", "101"])("refuses %j as a probability", (typed) => {
  expect(probability?.validate?.(typed)).toBeDefined();
});

it.each(["", "0", "40", "100"])("takes %j as a probability", (typed) => {
  expect(probability?.validate?.(typed)).toBeUndefined();
});
