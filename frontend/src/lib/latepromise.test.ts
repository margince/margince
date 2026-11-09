import { describe, expect, it } from "vitest";
import { UTC_ZONE } from "../format/timezone";
import { translate, translatePlural } from "../i18n";
import { en } from "../i18n/en";
import { latePromise } from "./latepromise";

const words = {
  t: (key: Parameters<typeof translate>[1]) => translate("en", key),
  plural: (
    base: Parameters<typeof translatePlural>[1],
    count: number,
    params?: Record<string, string>,
  ) => translatePlural("en", base, count, params),
  locale: "en" as const,
};

describe("latePromise", () => {
  it("says a promise due earlier today is overdue, in both places", () => {
    expect(
      latePromise(
        "2026-08-12T08:00:00Z",
        "2026-08-12T10:00:00Z",
        UTC_ZONE,
        "task",
        words,
      ),
    ).toEqual({
      badge: en["co.next.overdue"],
      sentence: en["promise.late.today"],
    });
  });

  it("words a claim as a promise made for a date", () => {
    expect(
      latePromise(
        "2026-08-11T08:00:00Z",
        "2026-08-12T10:00:00Z",
        UTC_ZONE,
        "claim",
        words,
      ),
    ).toEqual({
      badge: en["co.next.daysLate_one"].replace("{days}", "1"),
      sentence: en["promise.claim.sentence_one"],
    });
  });

  it("says nothing of a promise that is not late", () => {
    expect(
      latePromise(
        "2026-08-13T08:00:00Z",
        "2026-08-12T10:00:00Z",
        UTC_ZONE,
        "task",
        words,
      ),
    ).toBeUndefined();
  });
});
