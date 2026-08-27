import { describe, expect, it } from "vitest";
import { LOCALES } from "../../i18n";
import { onboardingLocale, PROMPTED_LOCALES } from "./onboarding-locale";

describe("onboarding conversation locale", () => {
  it("passes a prompted locale through untouched", () => {
    for (const locale of PROMPTED_LOCALES) {
      expect(onboardingLocale(locale)).toBe(locale);
    }
  });

  it("falls back to the contract default for a locale the prompts do not cover", () => {
    // No SHIPPED locale needs this today — the prompt library covers all three
    // — and the case stays because that is a fact about today, not a property.
    // A UI catalog ships as soon as its strings are translated; the prompt
    // library follows separately, and in the window between them sending the
    // catalog's code would 422 the reader out of onboarding.
    //
    // Cast, because the only codes left that reach this branch are ones the
    // Locale type does not admit — which is exactly the shape a fourth
    // language arrives in.
    expect(onboardingLocale("fr" as never)).toBe("en");
  });

  // Derived from LOCALES on one side and PROMPTED_LOCALES on the other, so a
  // locale added to either registry is covered here without editing this test.
  // The point is that NO shipped locale can put an unenumerated value on the
  // wire.
  it("maps every shipped locale to a value the contract enumerates", () => {
    for (const locale of LOCALES) {
      expect(PROMPTED_LOCALES, locale).toContain(onboardingLocale(locale));
    }
  });
});
