// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { CORE_OBJECTS } from "../../format/rbacobjects";
import { agentClientList } from "../../testing/agent-clients.fixtures";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { allowance, status } from "../../testing/ai-admin.testkit";
import { bookingProfile } from "../../testing/book.testkit";
import { formatFixture } from "../../testing/import.fixtures";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import { jsonResponse, type RouteMap } from "../../testing/story-utils";

type Schemas = components["schemas"];

// Every verb on every object, so the admin meets every card a page can draw.
// That includes cards gated on grants the page itself does not ask for.
export const EVERY_GRANT: GrantSpec = Object.fromEntries(
  Object.keys(CORE_OBJECTS).map((object) => [
    object,
    ["create", "read", "update", "delete"],
  ]),
);

// An unused installation's answers to the reads that are not lists: the empty
// page fallback lacks their required fields, and the card reading one throws.

const NOTIFICATION_PREFERENCES: Schemas["NotificationPreferenceList"] = {
  items: [
    { class: "approval_pending", delivery: "email", chosen: false },
    { class: "automation", delivery: "in_app", chosen: false },
    { class: "lead_sla", delivery: "in_app", chosen: false },
    { class: "capture", delivery: "in_app", chosen: false },
    { class: "system", delivery: "in_app", chosen: false },
    { class: "coach", delivery: "in_app", chosen: false },
  ],
};

const QUIET_CAPTURE: Schemas["CaptureActivityResponse"] = {
  funnel: { captured: 0, internal: 0, suppressed: 0, deferred: 0, fault: 0 },
  data: [],
  page: { next_cursor: null, has_more: false },
  payload_capture_enabled: false,
  window_hours: 24,
};

const CAPTURE_SETTINGS: Schemas["CaptureSettings"] = {
  auto_enrich: true,
  mail_sharing: true,
  shared_posture_allowed: false,
  signature_enrich: true,
  auto_enrich_daily_cap: 500,
  site_read: { max_pages: 60, max_mib: 32, wall_seconds: 240 },
  mail_sync_interval_seconds: 120,
  confidentiality_pass_cap: 40,
};

const INSTALLATION_SETTINGS: Schemas["InstallationSettings"] = {
  name: "Acme Inc",
  base_language: "en",
  fiscal_year_start_month: 1,
  sign_in_providers: [],
  max_upload_bytes: 25_000_000,
  dead_work_banner_hours: 24,
  oauth_access_token_ttl_minutes: 43_200,
  operations: defaultOperations,
  agent_allowances: defaultAgentAllowances,
  forecast_forward_measure: "commit_evidence",
  timezone: meFixture().user.timezone,
  base_currency: "EUR",
  date_format: "dmy",
  time_format: "24h",
  base_currency_locked: false,
  call_whatsapp: true,
  call_telegram: true,
  call_zalo: true,
  mcp_client_check: false,
};

// A fresh installation's port, read from its default.
const CONFIGURATION: Schemas["ConfigurationEntry"][] = [
  {
    key: "http.addr",
    env: "MARGINCE_ADDR",
    flag: "addr",
    type: "string",
    doc: "The address the api listens on.",
    roles: ["api"],
    secret: false,
    stored: false,
    value: ":8080",
    default: ":8080",
    origin: { source: "default", name: "" },
  },
];

const UNREAD_COMPANY: Schemas["CompanyProfile"] = {
  company_id: "00000000-0000-4000-8000-0000000000c1",
  display_name: "Acme Inc",
  fields: [],
};

export const COMPANY_CONTEXT_ON: Schemas["CompanyContextCapabilities"] = {
  rollout: "onboarding",
  read_enabled: true,
  tasks_enabled: true,
  onboarding_enabled: true,
};

export const COMPANY_CONTEXT_OFF: Schemas["CompanyContextCapabilities"] = {
  rollout: "off",
  read_enabled: false,
  tasks_enabled: false,
  onboarding_enabled: false,
};

const NO_JURISDICTION: Schemas["InstallationJurisdiction"] = {
  selected: null,
  available: [],
  floor: { known: false, from_year_end: false, packs: [] },
  deadlines: [],
  documents: [],
};

const UNUSED_AI: RouteMap = {
  "GET /ai/budget": () => jsonResponse(allowance),
  "GET /ai/status": () => jsonResponse(status),
  "GET /ai/provider-keys": () =>
    jsonResponse({ providers: [] } satisfies Schemas["AiProviderKeyList"]),
  "GET /ai/health": () =>
    jsonResponse({ window_hours: 1, rungs: [] } satisfies Schemas["AiHealth"]),
  "GET /ai/agent-usage": () =>
    jsonResponse({ resets_at: "2026-10-15T00:00:00Z", data: [] }),
};

const ROLE_DIRECTORY: RouteMap = {
  "GET /users/assignable-roles": () =>
    jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
  "GET /roles": () => jsonResponse({ roles: [] }),
};

export const NOTIFICATION_ROUTES: RouteMap = {
  "GET /me/notification-preferences": () =>
    jsonResponse(NOTIFICATION_PREFERENCES),
  "GET /me/brief-delivery": () => jsonResponse({}),
  "GET /teams": () => jsonResponse({ data: [], page: { has_more: false } }),
};

// Every page story routes all of these: a card can sit on more than one page.
// No vendor app yet: the answer the sign-in page reads for each vendor. The
// contract makes it complete, so an unrouted read would be an answer no server
// gives.
const NO_OAUTH_APP = (provider: string) => ({
  provider,
  configured: false,
  client_id: "",
  source: "none",
  redirect_uris: [],
});

export const UNUSED_INSTALLATION_ROUTES: RouteMap = {
  ...NOTIFICATION_ROUTES,
  ...UNUSED_AI,
  ...ROLE_DIRECTORY,
  "GET /capture/activity": () => jsonResponse(QUIET_CAPTURE),
  "GET /capture/activity/workspace": () => jsonResponse(QUIET_CAPTURE),
  "GET /capture/settings": () => jsonResponse(CAPTURE_SETTINGS),
  "GET /installation/oauth-apps/google": () =>
    jsonResponse(NO_OAUTH_APP("google")),
  "GET /installation/oauth-apps/microsoft": () =>
    jsonResponse(NO_OAUTH_APP("microsoft")),
  "GET /me/linkedin-reach": () =>
    jsonResponse({
      accounts: [],
      accounts_total: 0,
      unresolved_connections: 0,
    }),
  "GET /installation/agent-clients": () => jsonResponse(agentClientList([])),
  "GET /knowledge/corpora": () =>
    jsonResponse({ items: [] } satisfies Schemas["KnowledgeCorpusList"]),
  "GET /imports/format": () => jsonResponse(formatFixture),
  "GET /extensions": () => jsonResponse({ extensions: [] }),
  "GET /finance/source": () =>
    jsonResponse({ available: [] } satisfies Schemas["FinanceSourceState"]),
  "GET /installation/jurisdiction": () => jsonResponse(NO_JURISDICTION),
  "GET /installation/settings": () => jsonResponse(INSTALLATION_SETTINGS),
  "GET /installation/configuration": () => jsonResponse(CONFIGURATION),
  "GET /scheduling/profile": () => jsonResponse(bookingProfile),
  "GET /company": () => jsonResponse(UNREAD_COMPANY),
  "GET /company/context/capabilities": () => jsonResponse(COMPANY_CONTEXT_ON),
};
