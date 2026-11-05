import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi as vietnamese } from "../i18n/vi";
import {
  INDUSTRY_CODES,
  industryLabelKey,
  industryOptions,
  industryWords,
} from "./industry";

const catalogs = { en, de, vi: vietnamese };

describe("the industry list", () => {
  it("words every code in every language", () => {
    for (const [locale, catalog] of Object.entries(catalogs)) {
      for (const code of INDUSTRY_CODES) {
        const key = industryLabelKey(code);
        expect(key, `${locale} ${code}`).toBeDefined();
        expect(key && catalog[key], `${locale} ${code}`).toBeTruthy();
      }
    }
  });

  it("shows a stored value that is no code as it is", () => {
    expect(industryLabelKey("Medical devices")).toBeUndefined();
    expect(industryWords((key) => en[key], "Medical devices")).toBe(
      "Medical devices",
    );
    expect(industryWords((key) => en[key], "healthcare")).toBe("Healthcare");
    expect(industryWords((key) => de[key], "healthcare")).toBe(
      "Gesundheitswesen",
    );
  });

  it("offers the codes as options labelled in the reader's language", () => {
    const options = industryOptions((key) => vietnamese[key], "vi");
    expect(options).toHaveLength(INDUSTRY_CODES.length);
    expect(options.find((option) => option.value === "healthcare")?.label).toBe(
      "Y tế",
    );
  });
});
