import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { deferralDetail } from "./deferral-detail";

describe("deferral explanations", () => {
  it.each(["en", "de", "vi"] as const)(
    "explains the retry cause in %s",
    (locale) => {
      const t = (key: Parameters<typeof translate>[1]) =>
        translate(locale, key);
      for (const reason of ["provider", "budget"] as const) {
        expect(
          deferralDetail(t, {
            status: "deferred",
            deferral_reason: reason,
            status_detail: "Legacy retry marker",
          }),
        ).toBe(t(`aiWait.${reason}`));
      }
    },
  );
  it("preserves unknown and legacy guidance without guessing a cause", () => {
    const t = (key: Parameters<typeof translate>[1]) => translate("en", key);
    for (const reason of [undefined, "future_reason"]) {
      expect(
        deferralDetail(t, {
          status: "deferred",
          deferral_reason: reason,
          status_detail: "Original guidance",
        }),
      ).toBe("Original guidance");
    }
    expect(
      deferralDetail(t, {
        status: "failed",
        deferral_reason: "budget",
        status_detail: "Failure",
      }),
    ).toBe("Failure");
  });
});
