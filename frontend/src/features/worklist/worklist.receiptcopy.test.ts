import { expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
import { LOCALES, translate } from "../../i18n";
import { receiptBasis, receiptSummary } from "./worklist.receiptcopy";

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
    expect(result).toContain(
      state === "reversed" ? "Reverted close-date estimate" : "Close date:",
    );
    expect(result).not.toContain("needs confirmation");
    expect(result).not.toContain("Unverified");
  },
);

it("shows recorded estimate observations and admits a missing basis", () => {
  const receipt = {
    id: "r",
    kind: "close_date_correction",
    summary: "old",
    occurred_at: "2026-09-13T08:00:00Z",
    close_date_change: { date_changed: true, forecast_changed: false },
  };
  const t = (
    key: Parameters<typeof translate>[1],
    values?: Record<string, string>,
  ) => translate("en", key, values);
  expect(receiptBasis(receipt, t, "en")).toContain(
    "Estimate basis unavailable",
  );
  expect(
    receiptBasis(
      {
        ...receipt,
        close_date_change: {
          ...receipt.close_date_change,
          estimate_basis: {
            won_deals: 24,
            median_days_per_stage: 4.5,
            remaining_stages: 2,
          },
        },
      },
      t,
      "en",
    ),
  ).toBe(
    "Estimate from 24 won deals · Median days per stage: 4.5 · Stages remaining: 2. No buyer-confirmed date.",
  );
  expect(
    receiptBasis(
      { ...receipt, undo: { audit_log_id: "a", version: 1, reversed: true } },
      t,
      "en",
    ),
  ).toBeNull();
});
