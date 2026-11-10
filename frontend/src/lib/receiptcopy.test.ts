import { expect, it } from "vitest";
import { browserZone } from "../format/timezone";
import { LOCALES, translate } from "../i18n";
import { headlineUnder, receiptBasis, receiptSummary } from "./receiptcopy";

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
      browserZone(),
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
    browserZone(),
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
      browserZone(),
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

it("reports a decided approval by its sentence and the values it set", () => {
  const result = receiptSummary(
    {
      id: "receipt",
      kind: "update_record",
      summary: "Update record deal: name=Acme, Inc.",
      summary_parts: {
        sentence: "Update record deal",
        fields: [{ field: "name", value: "Acme, Inc." }],
        more_fields: 0,
      },
      occurred_at: "2026-09-13T08:00:00Z",
    },
    (key, values) => translate("en", key, values),
    "en",
    browserZone(),
  );
  expect(result).toBe("Update record deal — Name: Acme, Inc.");
});

it.each(LOCALES)(
  "names a close date the customer gave in its own words in %s, with no estimate basis",
  (locale) => {
    const receipt = {
      id: "receipt",
      kind: "deal_close_date",
      summary: "server summary",
      occurred_at: "2026-09-13T08:00:00Z",
      close_date_change: {
        date_changed: true,
        forecast_changed: false,
        before: "2026-09-12",
        after: "2026-10-12",
      },
    };
    const t = (
      key: Parameters<typeof translate>[1],
      values?: Record<string, string>,
    ) => translate(locale, key, values);
    const applied = receiptSummary(receipt, t, locale, browserZone());
    const undone = receiptSummary(
      { ...receipt, undo: { audit_log_id: "a", version: 2, reversed: true } },
      t,
      locale,
      browserZone(),
    );
    expect(applied).toContain(t("home.receipt.verdictDate").split(":")[0]);
    expect(undone).toContain(
      t("home.receipt.verdictDateReverted").split(":")[0],
    );
    for (const estimate of [
      t("home.receipt.dateUnconfirmed").split(":")[0],
      t("home.receipt.dateReverted").split(";")[0],
    ]) {
      expect(applied).not.toContain(estimate);
      expect(undone).not.toContain(estimate);
    }
    expect(receiptBasis(receipt, t, locale)).toBeNull();
  },
);

it.each([
  [
    'Changed the win probability on "Acme rollout": 50% → 0%',
    "Changed the win probability: 50% → 0%",
  ],
  [
    'Moved "Acme rollout" from Proposal to Negotiation. Probability now 60%',
    "Moved from Proposal to Negotiation. Probability now 60%",
  ],
  ['Reopened "Acme rollout" in Discovery', "Reopened in Discovery"],
  ["Logged a call", "Logged a call"],
])("reads %s without the deal it sits under", (headline, expected) => {
  const result = headlineUnder(
    {
      id: "receipt",
      kind: "deal_probability",
      summary: headline,
      headline,
      occurred_at: "2026-09-13T08:00:00Z",
      subject: { type: "deal", id: "deal", label: "Acme rollout" },
    },
    (key, values) => translate("en", key, values),
    "en",
    browserZone(),
  );
  expect(result).toBe(expected);
});
