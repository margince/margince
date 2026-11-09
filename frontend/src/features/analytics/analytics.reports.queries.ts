import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { Section } from "../../lib/analytics.address";
import { unwrap } from "../../lib/problem";
import { FIELD_CURRENCY } from "./analytics.cells";
import {
  type AnalyticsScope,
  reportScope,
  scopeKey,
} from "./analytics.context";

type ReportKey =
  | "pipeline-current"
  | "forecast"
  | "open-deals-per-company"
  | "projects-by-phase"
  | "project-commitments"
  | "projects-gone-quiet";

export type CardKey = ReportKey | "pipeline-by-close-month";

const CARD_REPORT: Record<CardKey, ReportKey> = {
  "pipeline-current": "pipeline-current",
  "pipeline-by-close-month": "pipeline-current",
  forecast: "forecast",
  "open-deals-per-company": "open-deals-per-company",
  "projects-by-phase": "projects-by-phase",
  "project-commitments": "project-commitments",
  "projects-gone-quiet": "projects-gone-quiet",
};

export const reportOf = (card: CardKey): ReportKey => CARD_REPORT[card];

export const SECTION_REPORTS = {
  overview: [],
  reports: [],
  targets: [],
  definitions: [],
  forecast: [],
  accounts: [],
  pipeline: [
    "pipeline-current",
    "pipeline-by-close-month",
    "forecast",
    "open-deals-per-company",
  ],
  performance: [],
  sdr: [],
  leads: [],
  partners: [],
  conversion: [],
  outcomes: [],
  coverage: [],
  delivery: ["project-commitments", "projects-gone-quiet", "projects-by-phase"],
  questions: [],
} as const satisfies Record<Section, readonly CardKey[]>;

const REPORT_GROUP_BY: Record<CardKey, string[]> = {
  "pipeline-current": ["stage_id"],
  "pipeline-by-close-month": ["close_month"],
  forecast: ["win_probability_band"],
  "open-deals-per-company": ["company_id", FIELD_CURRENCY],
  "projects-by-phase": [],
  "project-commitments": [],
  "projects-gone-quiet": [],
};

type ReportAggregate = NonNullable<
  components["schemas"]["RunReportRequest"]["aggregates"]
>[number];

const REPORT_AGGREGATES: Record<CardKey, ReportAggregate[]> = {
  "pipeline-by-close-month": [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
    { fn: "count", field: "amount_base_minor", as: "priced_deals" },
  ],
  "pipeline-current": [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
    { fn: "count", field: "amount_base_minor", as: "priced_deals" },
  ],
  forecast: [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
    { fn: "count", field: "amount_base_minor", as: "priced_deals" },
  ],
  "open-deals-per-company": [
    { fn: "sum", field: "amount_minor", as: "raw_minor" },
    { fn: "count", as: "deal_count" },
  ],
  "projects-by-phase": [],
  "project-commitments": [],
  "projects-gone-quiet": [],
};

export function useReportRun(
  card: CardKey,
  scope?: AnalyticsScope,
  enabled = true,
) {
  const report = reportOf(card);
  return useQuery({
    enabled,
    queryKey: ["report", card, scope ? scopeKey(scope) : null],
    queryFn: async () => {
      return unwrap(
        await api.POST("/reports/{report}", {
          params: { path: { report } },
          body: {
            scope: scope ? reportScope(scope) : undefined,
            group_by: REPORT_GROUP_BY[card],
            aggregates: REPORT_AGGREGATES[card],
          },
        }),
      );
    },
  });
}
