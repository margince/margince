import type { Envelope } from "../types";

/**
 * The answers read_forecast gives, shaped as each routed tool seals it.
 *
 * MIRRORED BY HAND from agents.ForecastReadingsResult,
 * agents.ForecastMovementResult and agents.ForecastAssuranceResult.
 */
export const readingsFixture: Envelope = {
  data: {
    period_start: "2026-10-01",
    period_end: "2026-12-31",
    scope_kind: "owner",
    won_minor: 4_000_000,
    evidence_minor: 6_500_000,
    best_case_minor: 9_000_000,
    open_minor: 12_000_000,
    weighted_minor: 7_300_000,
    eligible_count: 14,
    priced_count: 12,
    confirmed_date_count: 9,
    fx_missing_count: 0,
    as_of: "2026-10-30T07:00:00Z",
    base_currency: "EUR",
    coverage_note: "2 of 14 deals have no amount and are in no total.",
    snapshots: [],
  },
};

export const movementFixture: Envelope = {
  data: {
    reading: "weighted",
    opening_minor: 10_000_000,
    closing_minor: 11_200_000,
    buckets: [
      { name: "new", amount_minor: 2_000_000, deal_count: 2 },
      { name: "pushed_out", amount_minor: -1_000_000, deal_count: 1 },
      { name: "amount", amount_minor: 200_000, deal_count: 1 },
      { name: "fx", amount_minor: 0, deal_count: 0 },
    ],
    deals: [
      {
        deal_id: "0195c3a0-0000-7000-8000-0000000000d1",
        bucket: "new",
        amount_minor: 1_500_000,
      },
      {
        deal_id: "0195c3a0-0000-7000-8000-0000000000d2",
        bucket: "new",
        amount_minor: 500_000,
      },
      {
        deal_id: "0195c3a0-0000-7000-8000-0000000000d3",
        bucket: "pushed_out",
        amount_minor: -1_000_000,
      },
      {
        deal_id: "0195c3a0-0000-7000-8000-0000000000d4",
        bucket: "amount",
        amount_minor: 200_000,
      },
    ],
  },
};

export const checksFixture: Envelope = {
  data: {
    run_id: "0195c3a0-0000-7000-8000-0000000000c1",
    as_of: "2026-10-30T03:00:00Z",
    status: "complete",
    readiness: "ready_with_exceptions",
    eligible_deals: 14,
    sources: [],
  },
};
