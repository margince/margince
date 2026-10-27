// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Locale, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { DEAL_ROLES, roleFromTyped, signalKindLabel } from "./recordlabels";

const LOCALES: readonly Locale[] = ["en", "de", "vi"];

const wordsOf = (locale: Locale) => (key: MessageKey) => translate(locale, key);

describe("a deal role typed in a reader's words", () => {
  it.each(LOCALES)("goes as its key in any case and spacing (%s)", (locale) => {
    const t = wordsOf(locale);
    const label = t("co.roleLabel.economic_buyer");
    const shouted = ` ${label.toUpperCase().replaceAll(" ", "   ")} `;
    expect(roleFromTyped(shouted, t, DEAL_ROLES)).toBe("economic_buyer");
    expect(
      roleFromTyped(t("co.roleLabel.blocker").normalize("NFD"), t, DEAL_ROLES),
    ).toBe("blocker");
  });

  it.each(LOCALES)(
    "goes as its key when the key itself is typed (%s)",
    (locale) => {
      expect(roleFromTyped("Economic_Buyer", wordsOf(locale), DEAL_ROLES)).toBe(
        "economic_buyer",
      );
    },
  );

  // A seat the coverage rules name may sit outside the five built-in roles.
  it("goes as an offered role's key when its shown words are typed back", () => {
    const t = wordsOf("en");
    expect(
      roleFromTyped("Technical  Lead", t, DEAL_ROLES, ["technical_lead"]),
    ).toBe("technical_lead");
  });

  it("goes trimmed and as typed when it spells no role", () => {
    const t = wordsOf("vi");
    expect(roleFromTyped("  Người ủng hộ nội bộ ", t, DEAL_ROLES)).toBe(
      "Người ủng hộ nội bộ",
    );
    expect(roleFromTyped("", t, DEAL_ROLES)).toBe("");
  });
});

describe("signalKindLabel", () => {
  const t = (key: string) => `t:${key}`;

  it("words a core kind through the catalog", () => {
    expect(signalKindLabel("risk", t)).toBe("t:signal.kind.risk");
  });

  it("shows an extension kind by its own name, without the unit prefix", () => {
    expect(signalKindLabel("ext:finance-demo:ledger_loss", t)).toBe(
      "ledger loss",
    );
  });

  it("degrades an unknown kind to its own words", () => {
    expect(signalKindLabel("made_up_kind", t)).toBe("made up kind");
  });
});
