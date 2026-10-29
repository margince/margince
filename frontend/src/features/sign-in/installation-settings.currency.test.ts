// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { viewerZone } from "../../format/timezone";
import {
  catalogs,
  type Locale,
  translate,
  translatePlural,
  type Words,
} from "../../i18n";
import { currencyNote } from "../../lib/installation-settings.currency";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { defaultOperations } from "../../testing/operationsettings.fixtures";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type BaseCurrencyLock = components["schemas"]["BaseCurrencyLock"];

const SETTINGS: InstallationSettings = {
  name: "Brandt Automotive GmbH",
  timezone: viewerZone(),
  base_currency: "EUR",
  base_language: "en",
  fiscal_year_start_month: 1,
  dead_work_banner_hours: 24,
  oauth_access_token_ttl_minutes: 43_200,
  forecast_forward_measure: "commit_evidence",
  base_currency_locked: false,
  call_whatsapp: true,
  call_telegram: true,
  call_zalo: true,
  individual_performance: true,
  max_upload_bytes: 25_000_000,
  sign_in_providers: [],
  operations: defaultOperations,
  agent_allowances: defaultAgentAllowances,
};

function wordsFor(locale: Locale): Words {
  return {
    t: (key, params) => translate(locale, key, params),
    plural: (base, count, params) =>
      translatePlural(locale, base, count, params),
    locale,
  };
}

function locked(
  extra: Partial<InstallationSettings> = {},
): InstallationSettings {
  return { ...SETTINGS, base_currency_locked: true, ...extra };
}

describe("currencyNote words the lock facts in the reader's language", () => {
  const frozen = (count: number): BaseCurrencyLock => ({
    reason: "rates_frozen",
    count,
  });
  const sheet = (count: number): BaseCurrencyLock => ({
    reason: "rate_sheet",
    count,
    base_currency: "CHF",
  });
  const unnamed = (count: number): BaseCurrencyLock => ({
    reason: "rate_sheet",
    count,
  });
  const cases: [Locale, BaseCurrencyLock, string][] = [
    [
      "en",
      frozen(1),
      "1 record has frozen a conversion rate against this currency, so it cannot change.",
    ],
    [
      "en",
      frozen(3),
      "3 records have frozen a conversion rate against this currency, so it cannot change.",
    ],
    [
      "en",
      sheet(1),
      "1 currency rate converts into CHF. Clear the currency rates first to change the base currency.",
    ],
    [
      "en",
      sheet(3),
      "3 currency rates convert into CHF. Clear the currency rates first to change the base currency.",
    ],
    [
      "en",
      unnamed(1),
      "1 currency rate converts into a base currency that is not on record. Clear the currency rates first to change the base currency.",
    ],
    [
      "en",
      unnamed(3),
      "3 currency rates convert into a base currency that is not on record. Clear the currency rates first to change the base currency.",
    ],
    [
      "de",
      frozen(1),
      "Für 1 Eintrag ist bereits ein Wechselkurs zu dieser Währung festgeschrieben, daher ist sie nicht mehr änderbar.",
    ],
    [
      "de",
      frozen(3),
      "Für 3 Einträge ist bereits jeweils ein Wechselkurs zu dieser Währung festgeschrieben, daher ist sie nicht mehr änderbar.",
    ],
    [
      "de",
      sheet(1),
      "1 Wechselkurs rechnet in CHF um. Entferne zuerst die Wechselkurse, um die Basiswährung zu ändern.",
    ],
    [
      "de",
      sheet(3),
      "3 Wechselkurse rechnen in CHF um. Entferne zuerst die Wechselkurse, um die Basiswährung zu ändern.",
    ],
    [
      "de",
      unnamed(1),
      "1 Wechselkurs rechnet in eine Basiswährung um, die nicht hinterlegt ist. Entferne zuerst die Wechselkurse, um die Basiswährung zu ändern.",
    ],
    [
      "de",
      unnamed(3),
      "3 Wechselkurse rechnen in eine Basiswährung um, die nicht hinterlegt ist. Entferne zuerst die Wechselkurse, um die Basiswährung zu ändern.",
    ],
    [
      "vi",
      frozen(3),
      "3 hồ sơ đã cố định tỷ giá quy đổi theo loại tiền này, nên không thể thay đổi.",
    ],
    [
      "vi",
      sheet(3),
      "3 tỷ giá đang quy đổi sang CHF. Cần xóa các tỷ giá trước khi đổi loại tiền cơ sở.",
    ],
    [
      "vi",
      unnamed(3),
      "3 tỷ giá đang quy đổi sang một loại tiền cơ sở chưa được ghi nhận. Cần xóa các tỷ giá trước khi đổi loại tiền cơ sở.",
    ],
  ];
  for (const [locale, lock, sentence] of cases) {
    it(`${lock.reason}, ${lock.count}, ${lock.base_currency ?? "no base"}, in ${locale}`, () => {
      expect(
        currencyNote(locked({ base_currency_lock: lock }), wordsFor(locale)),
      ).toBe(sentence);
    });
  }

  it("groups a large count the reader's way", () => {
    const note = currencyNote(
      locked({ base_currency_lock: { reason: "rates_frozen", count: 1204 } }),
      wordsFor("de"),
    );
    expect(note).toContain("1.204");
  });

  it("never shows the server's English sentence when the facts are there", () => {
    const prose = "3 rate(s) in the sheet convert into EUR";
    const note = currencyNote(
      locked({
        base_currency_locked_reason: prose,
        base_currency_lock: {
          reason: "rate_sheet",
          count: 3,
          base_currency: "EUR",
        },
      }),
      wordsFor("de"),
    );
    expect(note).not.toContain(prose);
  });
});

describe("currencyNote falls back for an older or newer server", () => {
  const prose = "3 rate(s) in the sheet convert into EUR";

  it("shows the server's sentence when no facts are sent", () => {
    const note = currencyNote(
      locked({ base_currency_locked_reason: prose }),
      wordsFor("de"),
    );
    expect(note).toBe(prose);
  });

  it("shows the server's sentence for a reason this build does not know", () => {
    // A newer server's reason arrives over JSON, which no type narrows.
    const wire: InstallationSettings = JSON.parse(
      JSON.stringify({
        ...locked({ base_currency_locked_reason: prose }),
        base_currency_lock: { reason: "ledger_sealed", count: 2 },
      }),
    );
    expect(currencyNote(wire, wordsFor("en"))).toBe(prose);
  });

  it("shows the generic lock line when the server sends an empty reason", () => {
    expect(
      currencyNote(locked({ base_currency_locked_reason: "" }), wordsFor("en")),
    ).toBe(catalogs.en["installationSettings.baseCurrencyLocked"]);
  });

  it("shows the generic lock line when the server sends neither", () => {
    expect(currencyNote(locked(), wordsFor("vi"))).toBe(
      catalogs.vi["installationSettings.baseCurrencyLocked"],
    );
  });

  it("shows the hint while the currency is changeable", () => {
    expect(currencyNote(SETTINGS, wordsFor("en"))).toBe(
      catalogs.en["installationSettings.baseCurrencyHint"],
    );
  });
});
