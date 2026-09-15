// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
export const allowance: components["schemas"]["AiBudgetSnapshot"] = {
  config: { tokens_per_full_user: 12000000, company_monthly_tokens: null },
  revision: "budget-v1",
  eligible_full_users: 2,
  budgeted_full_users: 2,
  source: "per_user",
  monthly_tokens: 24000000,
  spent_tokens: 22453486,
  remaining_tokens: 1546514,
  band: "degraded",
  month_start_at: "2026-09-01T00:00:00Z",
  resets_at: "2026-10-01T00:00:00Z",
  observed_at: "2026-09-15T00:00:00Z",
};
export const feature: components["schemas"]["AiFeatureRoute"] = {
  task: "summarize",
  display_name: "Summarize correspondence",
  execution_mode: "interactive",
  leading_tier: "cheap_cloud",
  normal_candidates: [
    {
      tier: "cheap_cloud",
      provider: "gemini",
      model: "example-model",
      processing: "cloud_provider",
    },
  ],
  effective_candidates: [
    {
      tier: "local_small",
      provider: "gemini",
      model: "example-model",
      processing: "cloud_provider",
    },
  ],
  impact: "unchanged",
  budget_exempt: false,
};
export const status: components["schemas"]["AiStatus"] = {
  budget: allowance,
  observed_at: allowance.observed_at,
  routing_version: "routing-v1",
  task_contract_hash: "fixture",
  features: [feature],
  deferred_work: [
    { carrier: "site_read", unit: "reads", available: true, count: 3 },
    { carrier: "company_scan", unit: "scans", available: false },
    { carrier: "voice_build", unit: "builds", available: true, count: 0 },
  ],
  deferred_work_coverage: "durable_builds_and_scans",
};
