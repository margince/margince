// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";

// The reads behind Settings, AI and Settings, Maintenance. The catch-all's
// `{data,page}` is the wrong shape for each. A card reading a required field
// off it throws, and the throw takes the whole AI entry down. Every field below
// is required by its contract schema (AiUsage, AiCallListResponse, JobHealth,
// AiProviderKeyStatus).

// One provider keyed and one not, so the 390px and axe sweeps see both row
// states. `env_var`, `optional` and `credential_kind` are required by
// AiProviderKeyStatus.
export const aiProviderKeys = {
  providers: [
    {
      provider: "gemini",
      configured: true,
      env_var: "GEMINI_API_KEY",
      optional: false,
      credential_kind: "api_key",
    },
    {
      provider: "anthropic",
      configured: false,
      env_var: "ANTHROPIC_API_KEY",
      optional: false,
      credential_kind: "api_key",
    },
  ],
};

// One blocked provider, so the sweeps visit the health badge and the System
// health card in their failing state rather than only the empty one.
// `retry_after` is omitted for a degraded provider only.
export const aiProviderHealth = {
  configured: ["gemini", "anthropic"],
  providers: [
    {
      provider: "gemini",
      health: "out_of_credit",
      since: "2026-10-05T08:00:00Z",
      retry_after: "2026-10-05T08:15:00Z",
    },
  ],
};

// The lane bindings the routing card draws. `tiers` and `embeddings` are
// required by AiRouting; without them the form draws the unbound callout.
//
// Two chat tiers and the embedding lane, because the lane row is the widest
// thing on the page. Binding only chat would leave the retrieval row unvisited
// at 390px.
export const aiRouting = {
  profile: "eu_hosted",
  tiers: {
    cheap_cloud: { provider: "gemini", model: "gemini-2.5-flash" },
    premium: { provider: "anthropic", model: "claude-sonnet-4-5" },
  },
  embeddings: {
    provider: "ollama",
    model: "nomic-embed-text",
    base_url: "http://localhost:11434",
    dimensions: 768,
  },
};

// Whether the model lanes are answering (the AI settings health card). An
// empty `rungs` draws the empty state and never the table with its latency,
// failure count and sentinel. One healthy lane and one failing, so the axe
// pass measures both badge tones.
export const aiHealth = {
  window_hours: 1,
  rungs: [
    {
      tier: "cheap_cloud",
      healthy: true,
      calls: 412,
      failures: 3,
      last_call_at: "2026-07-13T09:41:00Z",
      median_latency_ms: 640,
    },
    {
      tier: "premium",
      healthy: false,
      calls: 7,
      failures: 7,
      last_sentinel: "provider_budget",
      last_call_at: "2026-07-13T09:12:00Z",
      median_latency_ms: 2_180,
    },
  ],
};

export const aiUsage = {
  days: [
    {
      date: "2026-07-04",
      tasks: [
        {
          task: "capture_classify",
          tier: "cheap_cloud",
          calls: 412,
          cached_hits: 96,
          tokens_in: 184_320,
          tokens_out: 21_460,
          cost_est_minor: 118,
        },
        {
          task: "enrich",
          tier: "premium",
          calls: 37,
          cached_hits: 2,
          tokens_in: 96_100,
          tokens_out: 44_820,
          cost_est_minor: 942,
        },
      ],
    },
    {
      date: "2026-07-05",
      tasks: [
        {
          task: "capture_classify",
          tier: "cheap_cloud",
          calls: 388,
          cached_hits: 104,
          tokens_in: 171_005,
          tokens_out: 19_884,
          cost_est_minor: 109,
        },
        {
          task: "summarize",
          tier: "local_small",
          calls: 51,
          cached_hits: 0,
          tokens_in: 60_240,
          tokens_out: 12_015,
          cost_est_minor: 0,
        },
      ],
    },
  ],
  budget: {
    monthly_tokens: 4_000_000,
    spent_tokens: 609_844,
    band: "normal",
    currency: "USD",
  },
};

export const aiUsageSummary = {
  cost_est_minor: aiUsage.days
    .flatMap((day) => day.tasks)
    .reduce((total, task) => total + task.cost_est_minor, 0),
  currency: aiUsage.budget.currency,
  computed_at: "2026-07-05T12:00:00Z",
} satisfies components["schemas"]["AiUsageSummary"];

// The call figures every provider row and tier popover reads. `rows` is
// required by AiCallStats. One row per provider and tier the fixtures above
// bind, with failures and timeouts, so the sweeps see the widest form of each
// line.
const callStatsRow = (key: string) => ({
  key,
  calls: 1_284,
  failed: 37,
  timeouts: 12,
  p50_ms: 940,
  p95_ms: 4_200,
  tokens_in: 1_284_000,
  tokens_out: 212_000,
  cost_microusd: 12_480_000,
  unpriced: 3,
});

export function aiCallStats(group: string | null) {
  const keys: Record<string, string[]> = {
    provider: aiProviderKeys.providers.map((p) => p.provider),
    tier: ["local_small", "cheap_cloud", "premium"],
  };
  return {
    window: "7d",
    group: group ?? "provider",
    rows: (keys[group ?? "provider"] ?? []).map(callStatsRow),
  };
}

// Two terminal calls, one clean and one that retried and degraded. The second
// puts a badge column and an error sentinel into the widest row, which a
// narrow viewport has to survive.
export const aiCalls = {
  data: [
    {
      id: "0d9f8c2e-6b41-4d2a-9a77-1f3c5b8e0a11",
      occurred_at: "2026-07-05T06:14:00Z",
      kind: "completion",
      task: "capture_classify",
      tier: "cheap_cloud",
      provider: "deepseek",
      model_id: "deepseek-chat",
      served_model: "deepseek-chat-0724",
      calls_attempted: 1,
      tokens_in: 1_284,
      tokens_out: 212,
      reasoning_tokens: 0,
      cached_tokens: 0,
      latency_ms: 940,
      cache_hit: false,
      degraded: false,
      error_sentinel: null,
      has_payload: true,
    },
    {
      id: "b71c4a55-2f08-4c93-8d61-77aa9e4c2b30",
      occurred_at: "2026-07-05T05:58:00Z",
      kind: "completion",
      task: "enrich",
      tier: "premium",
      provider: "anthropic",
      model_id: "claude-sonnet",
      served_model: "claude-sonnet-4-6",
      calls_attempted: 3,
      tokens_in: 12_940,
      tokens_out: 3_118,
      reasoning_tokens: 1_002,
      cached_tokens: 8_400,
      latency_ms: 7_310,
      cache_hit: true,
      degraded: true,
      error_sentinel: "provider_timeout",
      has_payload: false,
    },
  ],
  page: { next_cursor: null },
  payload_capture_enabled: true,
  tasks: ["capture_classify", "enrich", "summarize"],
};

// The German pack selected, with its classes, rule set and one document, so
// the page draws every section rather than the empty states alone.
export const installationJurisdiction: components["schemas"]["InstallationJurisdiction"] =
  {
    selected: "de",
    available: [
      {
        code: "de",
        unit_name: "eu",
        unit_version: "1.0.0",
        description: "German jurisdiction pack.",
        correspondence_floor: {
          name: "commercial_correspondence",
          keep: "P6Y",
          from_year_end: true,
        },
      },
      {
        code: "vn",
        unit_name: "apac",
        unit_version: "1.0.0",
        description: "Vietnamese jurisdiction pack.",
      },
    ],
    unit: {
      code: "de",
      unit_name: "eu",
      unit_version: "1.0.0",
      description: "German jurisdiction pack.",
    },
    retention_classes: [
      { name: "commercial_correspondence", keep: "P6Y", from_year_end: true },
    ],
    floor: {
      known: true,
      class: "commercial_correspondence",
      keep: "P6Y",
      from_year_end: true,
      packs: ["de", "vn"],
    },
    messaging: { version: 1, instruments: [] },
    deadlines: [
      { kind: "access", rule: "de v1", period: "P1M" },
      { kind: "rectify", rule: "de v1", period: "P1M" },
      { kind: "erasure", rule: "de v1", period: "P1M" },
    ],
    documents: [
      {
        id: "mitarbeiterinformation",
        title:
          "Beschäftigteninformation zur Erfassung von E-Mail-Korrespondenz",
        language: "de",
        audience: "employee",
        kind: "template",
        media_type: "text/markdown",
        size_bytes: 4474,
      },
    ],
  };

// A queue with something waiting, a dispatcher beside it, and one dead job.
// The dead count raises the card's alert, which an all-zero fixture would
// leave unmeasured.
export const jobHealth: components["schemas"]["JobHealth"] = {
  generated_at: "2026-07-05T06:20:00Z",
  dead_window_hours: 24,
  kinds: [
    {
      kind: "capture_ingest",
      queue: "default",
      fleet_wide: false,
      waiting: 12,
      running: 2,
      retrying: 1,
      dead: 0,
      dead_recent: 0,
      oldest_waiting_age_seconds: 195,
    },
    {
      kind: "enrich_company",
      queue: "enrich",
      fleet_wide: false,
      waiting: 0,
      running: 0,
      retrying: 0,
      dead: 2,
      dead_recent: 2,
      oldest_waiting_age_seconds: null,
    },
    {
      kind: "workspace_dispatch",
      queue: "dispatch",
      fleet_wide: true,
      waiting: 0,
      running: 1,
      retrying: 0,
      dead: 0,
      dead_recent: 0,
      oldest_waiting_age_seconds: null,
    },
  ],
  recent_failures: [
    {
      kind: "enrich_company",
      state: "discarded",
      attempt: 5,
      max_attempts: 5,
      failed_at: "2026-07-05T04:41:00Z",
      reason: "The provider refused the request.",
    },
    {
      kind: "capture_ingest",
      state: "retryable",
      attempt: 2,
      max_attempts: 5,
      failed_at: "2026-07-05T06:02:00Z",
      reason: "The mail provider was unreachable.",
    },
  ],
};

// A mailbox with a backlog and every sweep on schedule, so the sweeps measure
// the card's tables rather than its failed read.
export const captureHealth: components["schemas"]["CaptureHealth"] = {
  connection_concerns: [],
  generated_at: "2026-07-05T06:20:00Z",
  mailboxes: [
    {
      user_id: "u1",
      display_name: "Lena Fischer",
      contacts_awaiting_decision: 3,
      oldest_contact_age_seconds: 7_200,
      threads_awaiting_verdict: 0,
      oldest_thread_age_seconds: null,
    },
  ],
  classifier: { pending: 0, unsure: 0, exhausted: 0 },
  held_meetings: { count: 0 },
  sweeps: [
    {
      sweep: "settled_thread_verdicts",
      cadence_seconds: 3_600,
      last_succeeded_at: "2026-07-05T06:00:00Z",
      last_run: {
        outcome: "ok",
        finished_at: "2026-07-05T06:00:00Z",
        processed: 14,
        cap_hit: false,
      },
    },
    {
      sweep: "stranded_contacts",
      cadence_seconds: 3_600,
      last_succeeded_at: "2026-07-05T06:05:00Z",
      last_run: {
        outcome: "ok",
        finished_at: "2026-07-05T06:05:00Z",
        processed: 3,
        cap_hit: false,
      },
    },
    {
      sweep: "filed_meeting_holds",
      cadence_seconds: 900,
      last_succeeded_at: "2026-07-05T06:15:00Z",
      last_run: {
        outcome: "ok",
        finished_at: "2026-07-05T06:15:00Z",
        processed: 0,
        cap_hit: false,
      },
    },
  ],
};

export const recoveryHealth: components["schemas"]["RecoveryHealth"] = {
  generated_at: "2026-07-05T06:20:00Z",
  recovery_target_seconds: 14_400,
  data_loss_target_seconds: 3_600,
  last_drill: {
    started_at: "2026-07-01T02:00:00Z",
    finished_at: "2026-07-01T03:10:00Z",
    restored_to: "2026-07-01T01:45:00Z",
    outcome: "passed",
    operator: "Lena Fischer",
    notes: null,
    recovery_seconds: 4_200,
    data_loss_seconds: 900,
  },
};

export const extensionIngestHealth: components["schemas"]["ExtensionIngestHealth"] =
  {
    generated_at: "2026-07-05T06:20:00Z",
    window_days: 7,
    units: [],
  };
