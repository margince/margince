// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";
import type { GrantSpec } from "../../src/app/mefixture";
import type { MockProject } from "../projectmock";

// The booked meeting the contact record offers a brief for. Its id is the one
// the brief fixtures were written against, so the drawer's request and the
// answer describe the same room.
export const MEETING_ACTIVITY = "3f7c1a90-0000-4000-8000-00000000a001";

// The AC specs drive an admin. The UI scopes every write control on the grant
// map /me carries, not on the role name. Without real grants every button
// under test disappears.
//
// Listed explicitly rather than "everything": a spec that reaches a surface
// this list forgot fails loudly. Extend it when a new AC needs a new object.
export const E2E_ADMIN_GRANTS: GrantSpec = {
  report_definition: ["read"],
  reporting_framework: ["read"],
  // The four records the specs write: deals, leads, projects and the notes the
  // lead composer logs. A record page's write verbs ask the object grant and
  // the row's `writable` before they draw. No spec disqualifies or archives,
  // so the deletes stay unclaimed.
  deal: ["read", "create", "update"],
  lead: ["read", "create", "update"],
  project: ["read", "create", "update"],
  activity: ["read", "create"],
  automation: ["create", "read", "update", "delete"],
  pipeline: ["create", "read", "update", "delete"],
  custom_field: ["create", "read", "update", "delete"],
  webhook_subscription: ["create", "read", "update", "delete"],
  capture_settings: ["read", "update"],
  embedding_reindex: ["read", "update"],
  fx_rate: ["create", "read", "update"],
  ai_model_rate: ["create", "read", "update"],
  saved_view: ["create", "read", "update", "delete"],
  // The Voice DNA surface gates every write on this object, and the admin,
  // ops and manager roles hold all four. Without it the `settings/voice`
  // sweep measures a card with no controls.
  voice_profile: ["create", "read", "update", "delete"],
  // The installation's own profile (name, timezone, base currency), which the
  // company card gates every write on. The unsaved-guard suite needs it: that
  // card's draft outlives its dialog, so a reader can hold it while leaving.
  installation_settings: ["read", "update"],
  // The consent registry's gate: the server reads purposes under
  // `contact:read` (consent/store.go), not under a role. Read alone, because no
  // spec writes a contact from here. The Privacy & retention entry opens on
  // `privacy_request` below, and its card still needs this read.
  contact: ["read"],
  // Filters & views reads the vocabulary and previews under `list:read`
  // (collections/handlers.go). Saving a filter as a dynamic list is a
  // `list:create`. Without them the picker never loads, and the sweeps pass.
  list: ["create", "read", "update", "delete"],
  // `settings/knowledge` and `settings/license` are each gated on their own
  // read. Without it the sweep measures the access boundary instead.
  knowledge_corpus: ["create", "read", "update", "delete"],
  license: ["read"],
  // Model routing, which the AI settings page gates its whole entry on. Read
  // alone: the sweep does not measure the routing writes.
  ai_routing: ["read"],
  // The trail's own read, which `AuditLogCard` asks for. It is independent of
  // the `system_reset` grant below.
  audit_log: ["read"],
  // Emptying the installation. The page also needs the deployment to arm
  // `data_reset_available`, which the mocked /me does.
  system_reset: ["delete"],

  // The settings objects some card or catalog entry reads, checked against the
  // source. `oauth_application` is absent: `OAuthAppCard` still asks
  // `capture_settings:update`, and adding it here would hide that.
  //
  // The roster and team cards ask `user_admin` and `team_admin` verb by verb,
  // and the Members and Teams entries follow them.
  user_admin: ["read", "create", "update", "delete"],
  team_admin: ["read", "create", "update"],
  //
  // Extensions is two reads behind one flag (the unit inventory and every
  // role's grant on every object), and its toggles write through the update.
  role_admin: ["read", "update"],
  // The subject queue: read to open it, update to move a request through its
  // statuses. Creating one asks `contact:update`, which is why the contact grant
  // above stays read-only and no spec opens a request.
  privacy_request: ["read", "update"],
  job_health: ["read"],
  extension_access: ["read"],
  // AI usage, model calls and the health card, all three.
  ai_diagnostics: ["read"],
  ai_budget: ["read", "update"],
  // The purposes card's own verb. The read stays on `contact` above, the gate
  // the endpoint applies, and nothing here updates or deletes a purpose.
  consent_config: ["create"],
  // Read alone: the page opens on it, and the sign-in card's write checks
  // `installation_settings:update`, which this fixture holds.
  authentication_policy: ["read"],
  // `license:read` above opens the Seats page, so the card never reads this.
  // It is the grant a Management seat has instead of the licence.
  seat_usage: ["read"],

  // Tags is the shared vocabulary, Products the catalogue an offer is built
  // from, and Data import the run that brings records in. All are ordinary
  // sales surfaces, and the derived settings sweep opens each one.
  tag: ["create", "read", "update", "delete"],
  product: ["create", "read", "update", "delete"],
  offer_template: ["create", "read", "update", "delete"],
  import_run: ["create", "read", "update"],
};

// One coherent seed (Anna Weber, Brandt Automotive, the fleet-retrofit deal),
// so the explainer arithmetic reconciles across screens.

export const stages = [
  {
    id: "s1",
    workspace_id: "w",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "s2",
    workspace_id: "w",
    pipeline_id: "pl",
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 40,
  },
  {
    id: "s3",
    workspace_id: "w",
    pipeline_id: "pl",
    name: "Negotiation",
    position: 3,
    semantic: "open",
    win_probability: 60,
  },
  {
    id: "s4",
    workspace_id: "w",
    pipeline_id: "pl",
    name: "Won",
    position: 4,
    semantic: "won",
    win_probability: 100,
  },
  {
    id: "s5",
    workspace_id: "w",
    pipeline_id: "pl",
    name: "Lost",
    position: 5,
    semantic: "lost",
    win_probability: 0,
  },
];

// The one colleague every spec drives. `/users` (the roster page) and
// `/users/names` (the by-id name lookup) both read this array, so a seeded
// name cannot drift from the roster's own.
export const seats = [
  {
    id: "u1",
    email: "lena@seed.test",
    display_name: "Lena Fischer",
    status: "active",
    is_agent: false,
  },
];

// One working lead for the leads list and page. It is named, owned by u1,
// scored and promotable (it has an email), with the fields the inline rows edit.
export const seededLead = {
  id: "l-1",
  workspace_id: "w",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  title: "Head of Operations",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 46,
  owner_id: "u1",
  // The signed-in seat owns this lead, so the server sends writable: true.
  // Absent means not writable per the contract, and the page would refuse the
  // controls the specs press.
  writable: true,
  captured_by: "human:u1",
  source: "inbound",
  version: 3,
  created_at: "2026-07-01T08:00:00Z",
  updated_at: "2026-07-05T08:00:00Z",
};

export const anna = {
  id: "p-anna",
  workspace_id: "w",
  full_name: "Anna Weber",
  title: "Head of Procurement",
  emails: [{ id: "e1", email: "anna.weber@brandt.example", is_primary: true }],
  writable: true,
  captured_by: "connector:gmail",
  source: "gmail",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

export const brandt = {
  id: "o-brandt",
  workspace_id: "w",
  display_name: "Brandt Automotive GmbH",
  industry: "Automotive",
  size_band: "201-500",
  writable: true,
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

export const deals = [
  {
    id: "d-fleet",
    workspace_id: "w",
    name: "Fleet retrofit",
    amount_minor: 4_800_000,
    currency: "EUR",
    pipeline_id: "pl",
    stage_id: "s2",
    company_id: "o-brandt",
    // The seeded seat carries it, so the card's owner mark and the table's
    // owner column both have a colleague to name.
    owner_id: "u1",
    project_id: null as string | null,
    status: "open",
    // The reason a win carries when no signed agreement backs it. Declared
    // here because the deal patch map is typed from this literal.
    won_without_contract_reason: null as string | null,
    writable: true,
    stalled: true,
    // The row version, which the advance and the patch send back as their
    // precondition. Without one a write ships unpinned in the test.
    version: 2,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-05-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
    last_activity_at: "2026-05-01T08:00:00Z",
  },
  {
    id: "d-service",
    workspace_id: "w",
    name: "Service contract",
    amount_minor: 1_250_000,
    currency: "EUR",
    pipeline_id: "pl",
    stage_id: "s1",
    company_id: "o-brandt",
    project_id: null as string | null,
    status: "open",
    writable: true,
    stalled: false,
    version: 5,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-06-15T08:00:00Z",
    updated_at: "2026-06-20T08:00:00Z",
    last_activity_at: "2026-06-28T08:00:00Z",
  },
];

// The body of work the fleet deals are about. It starts in `initiative`,
// attached to no deal, and the project page and the deal form's project
// picker both read it.
export const seededProject: MockProject = {
  id: "pr-fleet",
  workspace_id: "w",
  name: "Flottenumbau Brandt",
  key: "BRANDT-FLEET",
  company_id: "o-brandt",
  owner_id: "u1",
  // The signed-in seat owns this project, so the server sends writable: true
  // and the page draws its write controls.
  writable: true,
  phase: "initiative",
  closed_reason: null,
  description: null,
  target_end_date: null,
  version: 1,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-05-01T08:00:00Z",
  last_activity_at: null,
};

// One persisted Morning-Brief run over the two seeded deals. It carries the
// composite score with its factors, so the Brief queue's arithmetic agrees
// with the deal amounts above.
export const briefRun = {
  id: "br-1",
  generated_at: "2026-07-05T05:30:00Z",
  as_of: "2026-07-05T05:00:00Z",
  candidate_count: 2,
  revenue_norm_minor: 4_800_000,
  items: [
    {
      id: "bi-1",
      deal_id: "d-fleet",
      rank: 1,
      composite: 0.74,
      feature_vector: {
        winnability: 0.4,
        revenue: 1,
        timing: 0.75,
        momentum: 1,
        warmth: 0.47,
      },
      evidence_ids: ["ev-1", "ev-2"],
      state: "new",
      // Written when the item is acted on or dismissed. The type admits a
      // timestamp rather than the bare `null` it starts at.
      state_at: null as string | null,
    },
    {
      id: "bi-2",
      deal_id: "d-service",
      rank: 2,
      composite: 0.41,
      feature_vector: {
        winnability: 0.2,
        revenue: 0.26,
        timing: 0.5,
        momentum: 0,
        warmth: 0.3,
      },
      evidence_ids: ["ev-3"],
      state: "new",
      state_at: null as string | null,
    },
  ],
};

export const approval = {
  id: "ap-1",
  workspace_id: "w",
  kind: "send_email",
  status: "pending",
  proposed_by: "agent:runner",
  summary: "Send the follow-up to Anna Weber",
  proposed_change: { subject: "Follow-up", body: "Hi Anna" },
  confidence: 0.62,
  evidence: [
    { evidence_snippet: "shall we sync next week?", source_type: "activity" },
  ],
  created_at: "2026-07-05T05:00:00Z",
};

// The closed automation starter library: two types with one
// integer parameter each. The editor derives its form from params_schema.
export const automationCatalog = [
  {
    key: "stalled_deal_nudge",
    name: "Stillstands-Erinnerung",
    description: "Staged a follow-up when a deal stalls.",
    trigger: "deal.stalled",
    action: "send_email",
    tier: "confirmation_required",
    params_schema: {
      type: "object",
      properties: {
        due_in_days: { type: "integer", minimum: 1, maximum: 30, default: 3 },
      },
      required: ["due_in_days"],
    },
  },
  {
    key: "task_on_stage_entry",
    name: "Aufgabe bei Phasenwechsel",
    description: "Creates a task when a deal enters a stage.",
    trigger: "deal.stage_changed",
    action: "create_task",
    tier: "auto_execute",
    params_schema: {
      type: "object",
      properties: {
        due_in_days: { type: "integer", minimum: 1, maximum: 30, default: 7 },
      },
      required: ["due_in_days"],
    },
  },
];

// The report answers the Analytics sections read, keyed by the report the
// screen asks for. Each result uses the aliases its report card requests.
export const reportFixtures: Record<string, unknown> = {
  // The Analytics stage table's own request: one converted row per stage,
  // under the aliases `REPORT_AGGREGATES` asks for, and a handle naming those
  // aggregates. The measured stage carries it; the second does not.
  "pipeline-current": {
    report: "pipeline-current",
    plan: { group_by: ["stage_id"] },
    columns: [
      "stage_id",
      "raw_minor",
      "weighted_minor",
      "deal_count",
      "priced_deals",
    ],
    rows: [
      {
        stage_id: "s1",
        raw_minor: 1_250_000,
        weighted_minor: 250_000,
        deal_count: 1,
        priced_deals: 1,
        derivation_url:
          "/v1/reports/pipeline-current/derivation?by=stage_id&agg=sum:amount_base_minor:raw_minor&agg=sum:weighted_base_minor:weighted_minor&agg=count::deal_count&agg=count:amount_base_minor:priced_deals&stage_id=s1",
      },
      {
        stage_id: "s2",
        raw_minor: 4_800_000,
        weighted_minor: 1_920_000,
        deal_count: 2,
        priced_deals: 2,
      },
    ],
    total_rows: 2,
    as_of: "2026-03-04T09:00:00Z",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
  },
  "projects-by-phase": {
    report: "projects-by-phase",
    plan: { group_by: ["phase"] },
    columns: [
      "phase",
      "projects",
      "open_deal_value_minor",
      "won_deal_value_minor",
    ],
    rows: [
      {
        phase: "delivering",
        projects: 3,
        open_deal_value_minor: 1_500_000,
        won_deal_value_minor: 8_000_000,
      },
    ],
    total_rows: 1,
    as_of: "2026-03-04T09:00:00Z",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
  },
  "project-commitments": {
    report: "project-commitments",
    plan: { group_by: ["project_id"] },
    // Name and phase travel with the id, because the card draws the project
    // as a link with its phase beside it.
    columns: [
      "project_id",
      "name",
      "phase",
      "open_commitments",
      "overdue_commitments",
    ],
    rows: [
      {
        project_id: "p1",
        name: "Rollout",
        phase: "delivering",
        open_commitments: 4,
        overdue_commitments: 1,
      },
    ],
    total_rows: 1,
    as_of: "2026-03-04T09:00:00Z",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
  },
  // Empty: "nothing has gone quiet" is a real answer, and the screen owes
  // words for it rather than an empty table.
  "projects-gone-quiet": {
    report: "projects-gone-quiet",
    plan: { group_by: ["project_id"] },
    columns: ["project_id", "quiet_since"],
    rows: [],
    total_rows: 0,
    as_of: "2026-03-04T09:00:00Z",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
  },
};

// What any drill-through handle resolves to: a definition, the source rows and
// the frame they were cut in. One record is masked out, so the notice that says
// so is on screen wherever the drawer is swept.
export const derivationFixture = {
  report: "pipeline-current",
  definition: "Anzahl offener Deals in der Phase Qualify",
  plan: { group_by: ["stage_id"] },
  columns: ["label", "amount_base_minor"],
  rows: [
    {
      label: "Brandt Automotive, Flottenumrüstung",
      amount_base_minor: 1_250_000,
    },
    { label: "BÄR Pharma, Verpackungsprüfung", amount_base_minor: 480_000 },
  ],
  total_rows: 2,
  excluded_by_permission: 1,
  as_of: "2026-03-04T09:00:00Z",
  as_of_pinned: true,
};

// The wire carries no origin, so this row stands in for the agent-authored
// case and renders like any other.
export const seededAutomation = {
  id: "au-1",
  key: "task_on_stage_entry",
  name: "Aufgabe nach Phasenwechsel",
  status: "enabled",
  params: { due_in_days: 7 },
  version: 3,
  created_at: "2026-06-20T08:00:00Z",
};

// The reader's saved views. Two are filters the Filters and views library
// lists, naming fields the vocabularies hold. Only v-fleet's is the filter
// the preview authors, so v-owned previews empty.
//
// v-partner is a deals list's whole state, and it names the non-default
// pipeline: pressing its tab has to restore the pipeline it was saved on.
export const seededViews: components["schemas"]["SavedView"][] = [
  {
    id: "v-owned",
    resource: "contacts",
    name: "Contacts I own",
    owner_id: "u1",
    shared_scope: "private",
    query: {
      filter: { and: [{ field: "owner_id", op: "eq", value: "u1" }] },
    },
    version: 1,
  },
  {
    id: "v-fleet",
    resource: "companies",
    name: "Fleet companies",
    owner_id: "u1",
    shared_scope: "private",
    query: {
      filter: { and: [{ field: "industry", op: "eq", value: "automotive" }] },
    },
    version: 1,
  },
  {
    id: "v-partner",
    resource: "deals",
    name: "Partner deals",
    owner_id: "u1",
    shared_scope: "private",
    query: {
      list: {
        q: "",
        sort: "",
        includeArchived: false,
        filters: { pipeline_id: "pl-partner" },
      },
    },
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
];

export const passports = [
  {
    id: "pp-1",
    label: "Marcus' Claude",
    scopes: ["read", "draft"],
    created_at: "2026-06-01T08:00:00Z",
    expires_at: "2026-08-01T08:00:00Z",
    last_used_at: "2026-07-04T18:00:00Z",
    revoked_at: null,
  },
  {
    id: "pp-2",
    label: "Alter Runner",
    scopes: ["read"],
    created_at: "2026-05-01T08:00:00Z",
    expires_at: "2026-06-01T08:00:00Z",
    last_used_at: null,
    revoked_at: "2026-05-20T08:00:00Z",
  },
];

// actor_id carries the typed principal id the way storekit stamps it:
// "human:<uuid>", "agent:<id>" or "connector:<name>". The read path resolves
// the display names. The server never writes an unprefixed id.
export const auditEntries = [
  {
    id: "al-1",
    workspace_id: "w",
    actor_type: "human",
    actor_id: "human:u1",
    actor_name: "Lena Fischer",
    action: "update",
    entity_type: "deal",
    entity_id: "d-fleet",
    occurred_at: "2026-07-05T07:00:00Z",
  },
  {
    // An agent under another human's authority, so the row reads as that
    // human rather than as the viewer. The actor-filter assertion tells "Du"
    // from a named teammate by it.
    id: "al-2",
    workspace_id: "w",
    actor_type: "agent",
    actor_id: "agent:runner",
    passport_id: "pp-1",
    on_behalf_of: "u2",
    on_behalf_of_name: "Marcus Brandt",
    action: "send_email",
    entity_type: "activity",
    entity_id: "01a11ea4-10cb-7576-ad76-0e5a3f9bbfae",
    occurred_at: "2026-07-05T06:00:00Z",
  },
  {
    // A bare connector: no grant presented, so no human to name and no gap to
    // report. Its own id is what a reader gets.
    id: "al-3",
    workspace_id: "w",
    actor_type: "connector",
    actor_id: "connector:gmail",
    action: "create",
    entity_type: "contact",
    entity_id: "p-anna",
    occurred_at: "2026-07-05T05:00:00Z",
  },
];

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

// A queue with something waiting, a dispatcher beside it, and one dead job.
// The dead count raises the card's alert, which an all-zero fixture would
// leave unmeasured.
export const jobHealth = {
  generated_at: "2026-07-05T06:20:00Z",
  kinds: [
    {
      kind: "capture_ingest",
      queue: "default",
      fleet_wide: false,
      waiting: 12,
      running: 2,
      retrying: 1,
      dead: 0,
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

export const publicSlots = [
  { start: "2026-07-06T09:00:00Z", end: "2026-07-06T09:30:00Z" },
  { start: "2026-07-06T10:00:00Z", end: "2026-07-06T10:30:00Z" },
];

// The knowledge page's document sets. The screen reads `items`, which the
// catch-all's `{data, page}` lacks, and a page that threw scores zero axe
// violations.
export const knowledgeCorpora = {
  items: [
    {
      id: "00000000-0000-4000-8000-0000000000a1",
      name: "Everything",
      topic_statement:
        "What this company sells, who it sells to, and what it has already said.",
      min_similarity: 0.35,
      default_ask: true,
      coverage: {
        documents_total: 42,
        chunks_total: 1_180,
        chunks_embedded: 1_180,
      },
      created_at: "2026-08-01T00:00:00Z",
    },
  ],
};

// The installation's licence, inside its grant. An envelope with no `state`
// renders the card's unlicensed arm with blank figures.
export const installationLicense = {
  state: "valid",
  seats_used: 9,
  seats_granted: 10,
  over_limit: false,
  checked_at: "2026-08-20T09:00:00Z",
};

// Settings, Capture activity, both scopes. `funnel` is required by
// CaptureActivityResponse, and the window reads it straight into its counters.
//
// One entry per outcome, so each of the funnel's five counters has a row and
// the filter has something to narrow. `payload_capture_enabled` is true: the
// counterparty and subject columns exist only when it is.
export const captureActivity = {
  funnel: {
    captured: 3,
    internal: 1,
    suppressed: 1,
    deferred: 1,
    fault: 1,
  },
  data: [
    {
      id: "11111111-1111-4111-8111-111111111111",
      connector: "gmail",
      outcome: "captured",
      counterparty: "anna.weber@brandt.example",
      subject: "Angebot zur Flottenerneuerung",
      occurred_at: "2026-08-28T07:12:00Z",
      activity_id: "22222222-2222-4222-8222-222222222222",
    },
    {
      id: "11111111-1111-4111-8111-111111111112",
      connector: "gmail",
      outcome: "internal",
      reason: "internal_only",
      counterparty: "lars@brandt.example",
      subject: "Re: Wochenplanung",
      occurred_at: "2026-08-28T06:40:00Z",
    },
    {
      id: "11111111-1111-4111-8111-111111111113",
      connector: "gmail",
      outcome: "suppressed",
      reason: "noise_prior",
      counterparty: "newsletter@example.com",
      subject: "Ihr woechentlicher Marktbericht",
      occurred_at: "2026-08-28T05:55:00Z",
    },
    {
      id: "11111111-1111-4111-8111-111111111114",
      connector: "telegram",
      outcome: "deferred",
      reason: "no_granting_human",
      counterparty: "+49 170 0000000",
      subject: null,
      occurred_at: "2026-08-28T05:10:00Z",
      resolution: {
        status: "pending",
        kind: null,
        resolved_at: null,
      },
    },
    {
      id: "11111111-1111-4111-8111-111111111115",
      connector: "gmail",
      outcome: "fault",
      reason: "derivation_failed",
      counterparty: null,
      subject: null,
      occurred_at: "2026-08-28T04:20:00Z",
    },
  ],
  page: { next_cursor: null },
  payload_capture_enabled: true,
  window_hours: 24,
};
