import { expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import { HOME_REPAIR_REASON_LABELS } from "./homerepair";

it("translates every mapped repair reason in every locale", () => {
  expect(Object.keys(HOME_REPAIR_REASON_LABELS).length).toBeGreaterThan(0);
  for (const [reason, key] of Object.entries(HOME_REPAIR_REASON_LABELS)) {
    for (const locale of LOCALES) {
      const text = translate(locale, key);
      expect(text, `${reason} (${locale})`).not.toBe(key);
      expect(text, `${reason} (${locale})`).toMatch(/\p{L}/u);
    }
  }
});
