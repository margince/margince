import { describe, expect, it } from "vitest";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import {
  healthCodes,
  healthImpacts,
  healthRecovery,
  healthSourceLabels,
} from "./systemhealth.copy";

// The receipts check serves every jurisdiction that owes an opt-out receipt,
// so none of its words names one country.

const keys: readonly (MessageKey | undefined)[] = [
  healthSourceLabels.receipts,
  healthCodes.receipts_sent,
  healthCodes.receipts_missed,
  healthRecovery.receipts,
  healthImpacts.receipts,
  "health.effect.receipts",
];

describe("the opt-out receipts health copy", () => {
  it.each([
    ["en", en],
    ["de", de],
    ["vi", vi],
  ])(
    "names no country in %s",
    (_, catalog: Readonly<Record<MessageKey, string>>) => {
      for (const key of keys) {
        expect(key).toBeDefined();
        if (!key) continue;
        expect(catalog[key]).not.toMatch(/vietnam|việt nam/i);
      }
    },
  );
});
