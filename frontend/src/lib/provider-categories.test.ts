import { describe, expect, it } from "vitest";
import type { MessageKey } from "../i18n/en";
import { categoryNames, categoryNamesTogether } from "./provider-categories";

// Two ways to name several categories, and the difference is what a reader
// does with them. A list is what a run asked for; a conjunction is what one
// press buys as one purchase. The buy button used the list, and a rep read
// "Buy work email, mobile number · 2 credits" as the work-email button — then
// got a mobile number he had not meant to ask for.

// The real message keys, so a renamed key surfaces here rather than passing on
// a stub that agrees with itself.
const NAMES: Partial<Record<MessageKey, string>> = {
  "provider.category.professionalEmail": "work email",
  "provider.category.mobile": "mobile number",
};

const t = (key: MessageKey): string => NAMES[key] ?? key;

const PAIR = ["professional_email", "mobile"];

describe("naming the categories one press buys", () => {
  it("joins them with the locale's conjunction, not a comma", () => {
    const label = categoryNamesTogether(PAIR, t, "en");
    expect(label).toBe("work email and mobile number");
    // Its own expectation, because a comma is precisely the spelling that
    // misread as a list: a change back to join(", ") must fail here rather
    // than merely look different.
    expect(label).not.toContain(",");
  });

  it("uses the reader's own language, not an English 'and' translated", () => {
    expect(categoryNamesTogether(PAIR, t, "de")).toBe(
      "work email und mobile number",
    );
    expect(categoryNamesTogether(PAIR, t, "vi")).toBe(
      "work email và mobile number",
    );
  });

  it("says just the one name when a press buys one thing", () => {
    expect(categoryNamesTogether(["professional_email"], t, "en")).toBe(
      "work email",
    );
  });

  it("leaves the list spelling alone, because a list is not a purchase", () => {
    // categoryNames still serves "what came back empty" and "what we never
    // asked for", where the items are separate facts rather than one buy.
    expect(categoryNames(PAIR, t)).toBe("work email, mobile number");
  });
});
