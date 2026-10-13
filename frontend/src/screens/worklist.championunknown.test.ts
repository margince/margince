// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A deal-risk row about a deal nobody recorded a champion for, and one about an
// imported deal named by its company and stage.

import { describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import type { Locale, Translator } from "../i18n";
import { translate } from "../i18n";
import { itemTitle, reasonText } from "./worklist.copy";
import type { WorklistItem, WorklistReason } from "./worklist.queries";

const zone = viewerZone();
const unknown: WorklistReason = { kind: "champion_unknown" };
const uncovered: WorklistReason = { kind: "no_champion" };

function inLocale(locale: Locale): Translator {
  return (key, params) => translate(locale, key, params);
}

describe("reasonText — champion unknown", () => {
  it.each([
    ["en", "champion unknown"],
    ["de", "Champion unbekannt"],
    ["vi", "chưa rõ người ủng hộ"],
  ] as const)("says the champion is unknown in %s", (locale, phrase) => {
    expect(reasonText(unknown, inLocale(locale), locale, zone)).toBe(phrase);
  });

  it("never reads as the finding that nobody is the champion", () => {
    const t = inLocale("en");
    expect(reasonText(unknown, t, "en", zone)).not.toBe(
      reasonText(uncovered, t, "en", zone),
    );
  });
});

describe("itemTitle — a deal named by its company and stage", () => {
  // The server names the deal and its subject alike, so the row says the
  // fallback once instead of printing the import key beside it.
  it("does not repeat a subject label the title already is", () => {
    const named = "Acme Logistics · Negotiation";
    const deal = "0199a0e8-617c-74ec-a4da-b41010b2a5b0";
    const item: WorklistItem = {
      id: deal,
      source: "deal_at_risk",
      category: "deals_at_risk",
      level: 2,
      consequence: "deal_drifts",
      title: named,
      subject: { type: "deal", id: deal, label: named },
      because: [unknown],
      actions: ["open"],
    };
    expect(itemTitle(item, inLocale("en"), "en")).toBe(named);
  });
});
