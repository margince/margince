// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Locale, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { roleFromTyped } from "./recordlabels";

const LOCALES: readonly Locale[] = ["en", "de", "vi"];

const wordsOf = (locale: Locale) => (key: MessageKey) => translate(locale, key);

describe("a deal role typed in a reader's words", () => {
  it.each(LOCALES)("goes as its key in any case and spacing (%s)", (locale) => {
    const t = wordsOf(locale);
    const label = t("co.roleLabel.economic_buyer");
    const shouted = ` ${label.toUpperCase().replaceAll(" ", "   ")} `;
    expect(roleFromTyped(shouted, t)).toBe("economic_buyer");
    expect(roleFromTyped(t("co.roleLabel.blocker").normalize("NFD"), t)).toBe(
      "blocker",
    );
  });

  it.each(LOCALES)(
    "goes as its key when the key itself is typed (%s)",
    (locale) => {
      expect(roleFromTyped("Economic_Buyer", wordsOf(locale))).toBe(
        "economic_buyer",
      );
    },
  );

  // A seat the coverage rules name may sit outside the five built-in roles.
  it("goes as an offered role's key when its shown words are typed back", () => {
    const t = wordsOf("en");
    expect(roleFromTyped("Technical  Lead", t, ["technical_lead"])).toBe(
      "technical_lead",
    );
  });

  it("goes trimmed and as typed when it spells no role", () => {
    const t = wordsOf("vi");
    expect(roleFromTyped("  Người ủng hộ nội bộ ", t)).toBe(
      "Người ủng hộ nội bộ",
    );
    expect(roleFromTyped("", t)).toBe("");
  });
});
