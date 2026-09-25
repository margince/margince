// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../src/api/schema";

// Keep the prospective readings aligned with the bindings and usage the same
// mocked installation serves; a list-page fallback is not a status response.
export function aiAdminFixture(
  tiers: Record<string, components["schemas"]["AiTierBinding"]>,
  usage: { monthly_tokens: number; spent_tokens: number },
): components["schemas"]["AiStatus"] {
  const budget: components["schemas"]["AiBudgetSnapshot"] = {
    config: {
      tokens_per_full_user: 12000000,
      company_monthly_tokens: usage.monthly_tokens,
    },
    revision: "budget-v1",
    eligible_full_users: 2,
    budgeted_full_users: 2,
    source: "company_override",
    monthly_tokens: usage.monthly_tokens,
    spent_tokens: usage.spent_tokens,
    remaining_tokens: usage.monthly_tokens - usage.spent_tokens,
    band: "normal",
    month_start_at: "2026-07-01T00:00:00Z",
    resets_at: "2026-08-01T00:00:00Z",
    observed_at: "2026-07-13T10:00:00Z",
  };
  return {
    budget,
    observed_at: budget.observed_at,
    routing_version: "routing-v1",
    task_contract_hash: "fixture",
    features: Object.entries(tiers).map(([tier, binding]) => {
      const candidate: components["schemas"]["AiRouteCandidate"] = {
        tier,
        provider: binding.provider,
        model: binding.model,
        processing: "cloud_provider",
      };
      return {
        task: tier === "premium" ? "enrich" : "capture_classify",
        display_name:
          tier === "premium" ? "Research accounts" : "Classify correspondence",
        execution_mode: "background",
        leading_tier: tier,
        normal_candidates: [candidate],
        effective_candidates: [candidate],
        impact: "unchanged",
        budget_exempt: false,
        decision_first: false,
      };
    }),
    deferred_work: [
      {
        carrier: "site_read",
        available: true,
        count: 0,
        unit: "website_reads",
      },
      {
        carrier: "company_scan",
        available: true,
        count: 0,
        unit: "account_scans",
      },
      {
        carrier: "voice_build",
        available: true,
        count: 0,
        unit: "voice_builds",
      },
    ],
    deferred_work_coverage: "durable_builds_and_scans",
    unused_tiers: [],
  };
}
