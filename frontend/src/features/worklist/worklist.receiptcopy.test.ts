import { expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
import { LOCALES, translate } from "../../i18n";
import { receiptSummary } from "./worklist.receiptcopy";

it.each(LOCALES)(
  "does not repeat unverified historical forecast prose in %s",
  (locale) => {
    const result = receiptSummary(
      {
        id: "receipt",
        kind: "close_date_change",
        summary: "Legacy summary",
        occurred_at: "2026-09-13T08:00:00Z",
        close_date_change: {
          date_changed: true,
          forecast_changed: false,
          before: "2026-09-12",
          after: "2026-10-12",
          basis:
            "Deals usually take 10 days per stage, guessed without observations.",
        },
      },
      (key, values) => translate(locale, key, values),
      locale,
      viewerZone(),
    );
    expect(result).not.toContain("usually");
    expect(result).not.toContain("guessed");
    expect(result).not.toContain("Legacy summary");
    expect(result).toContain(
      locale === "en"
        ? "needs confirmation"
        : locale === "de"
          ? "Bestätigung nötig"
          : "cần xác nhận",
    );
  },
);

it("does not reinterpret an unrelated business receipt", () => {
  const result = receiptSummary(
    {
      id: "receipt",
      kind: "approval",
      summary: "Won the deal",
      occurred_at: "2026-09-13T08:00:00Z",
    },
    (key, values) => translate("en", key, values),
    "en",
    viewerZone(),
  );
  expect(result).toBe("Won the deal");
});

it.each(["accepted", "reversed"])(
  "does not request confirmation again after a correction is %s",
  (state) => {
    const result = receiptSummary(
      {
        id: "receipt",
        kind: "close_date_change",
        summary: "Legacy summary",
        occurred_at: "2026-09-13T08:00:00Z",
        close_date_change: {
          date_changed: true,
          forecast_changed: false,
          before: "2026-09-12",
          after: "2026-10-12",
          basis: "Unverified legacy explanation",
        },
        review: {
          kind: "close_date",
          accepted: state === "accepted",
          reversed: state === "reversed",
          version: 1,
          can_undo: false,
          can_accept: false,
          writable: true,
        },
      },
      (key, values) => translate("en", key, values),
      "en",
      viewerZone(),
    );
    expect(result).toContain("Close date:");
    expect(result).not.toContain("needs confirmation");
    expect(result).not.toContain("Unverified");
  },
);
