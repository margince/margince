// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { defaultAgentAllowances } from "../features/automations/agentallowances.fixtures";
import { viewerZone } from "../format/timezone";
import {
  catalogs,
  isMessageKey,
  type Locale,
  translate,
  translatePlural,
  type Words,
} from "../i18n";
import { currencyNote } from "./installation-settings.currency";
import { defaultOperations } from "./operationsettings.fixtures";

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
  max_upload_bytes: 25_000_000,
  sign_in_providers: [],
  operations: defaultOperations,
  agent_allowances: defaultAgentAllowances,
};

// The arm each count takes: English and German split at one, Vietnamese has
// one form for every count.
const ARM: Record<Locale, Record<number, "one" | "other">> = {
  en: { 1: "one", 3: "other" },
  de: { 1: "one", 3: "other" },
  vi: { 1: "other", 3: "other" },
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

function expected(locale: Locale, key: string, params: Record<string, string>) {
  if (!isMessageKey(key)) {
    throw new Error(`the catalog has no key ${key}`);
  }
  let text = catalogs[locale][key];
  for (const [name, value] of Object.entries(params)) {
    text = text.replaceAll(`{${name}}`, value);
  }
  return text;
}

const PREFIX = "installationSettings.baseCurrencyLock";

describe("currencyNote words the lock facts in the reader's language", () => {
  const cases: {
    name: string;
    lock: (count: number) => BaseCurrencyLock;
    base: string;
    params: Record<string, string>;
  }[] = [
    {
      name: "frozen rates",
      lock: (count) => ({ reason: "rates_frozen", count }),
      base: `${PREFIX}.ratesFrozen`,
      params: {},
    },
    {
      name: "a rate sheet naming its base",
      lock: (count) => ({ reason: "rate_sheet", count, base_currency: "CHF" }),
      base: `${PREFIX}.rateSheet`,
      params: { base: "CHF" },
    },
    {
      name: "a rate sheet with no stored base",
      lock: (count) => ({ reason: "rate_sheet", count }),
      base: `${PREFIX}.rateSheetUnnamed`,
      params: {},
    },
  ];
  for (const locale of ["en", "de", "vi"] as const) {
    for (const count of [1, 3]) {
      for (const c of cases) {
        it(`${c.name}, ${count}, in ${locale}`, () => {
          const note = currencyNote(
            locked({ base_currency_lock: c.lock(count) }),
            wordsFor(locale),
          );
          expect(note).toBe(
            expected(locale, `${c.base}_${ARM[locale][count]}`, {
              ...c.params,
              count: String(count),
            }),
          );
        });
      }
    }
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
