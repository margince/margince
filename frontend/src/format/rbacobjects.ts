// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type CoreRbacObject = components["schemas"]["RbacObject"];

/** The areas a role's grants are read in, in the order the grid draws them. */
export const GRANT_AREAS = [
  "records",
  "sales",
  "analytics",
  "data",
  "ai",
  "people",
  "company",
  "governance",
] as const;

export type GrantArea = (typeof GRANT_AREAS)[number];

export const GRANT_AREA_NAME: Readonly<Record<GrantArea, MessageKey>> = {
  records: "nav.group.records",
  sales: "settings.group.sales",
  analytics: "nav.analytics",
  data: "settings.group.data",
  ai: "settings.group.ai",
  people: "settings.group.people",
  company: "settings.group.company",
  governance: "settings.group.governance",
};

export type CoreObjectName = Readonly<{ area: GrantArea; label: MessageKey }>;

// Keyed by the contract enum, which backend/gates/rbacvocabulary_test.go holds
// equal to the server's set, so an object added there fails this compile.
// Each area holds its objects in reading order.
export const CORE_OBJECTS: Readonly<Record<CoreRbacObject, CoreObjectName>> = {
  contact: { area: "records", label: "users.access.object.contact" },
  company: { area: "records", label: "users.access.object.company" },
  lead: { area: "records", label: "users.access.object.lead" },
  deal: { area: "records", label: "users.access.object.deal" },
  project: { area: "records", label: "users.access.object.project" },
  activity: { area: "records", label: "roles.object.activity" },
  relationship: { area: "records", label: "roles.object.relationship" },
  partner: { area: "records", label: "roles.object.partner" },
  signal: { area: "records", label: "roles.object.signal" },
  introduction: { area: "records", label: "roles.object.introduction" },
  list: { area: "records", label: "roles.object.list" },
  saved_view: { area: "records", label: "roles.object.saved_view" },

  pipeline: { area: "sales", label: "roles.object.pipeline" },
  product: { area: "sales", label: "roles.object.product" },
  offer: { area: "sales", label: "roles.object.offer" },
  offer_template: { area: "sales", label: "roles.object.offer_template" },
  contract: { area: "sales", label: "roles.object.contract" },
  commission: { area: "sales", label: "roles.object.commission" },
  deal_room: { area: "sales", label: "roles.object.deal_room" },
  forecast: { area: "sales", label: "roles.object.forecast" },
  weekly_plan: { area: "sales", label: "roles.object.weekly_plan" },
  tag: { area: "sales", label: "roles.object.tag" },
  custom_field: { area: "sales", label: "roles.object.custom_field" },
  computed_field: { area: "sales", label: "roles.object.computed_field" },

  report_definition: {
    area: "analytics",
    label: "roles.object.report_definition",
  },
  report_schedule: { area: "analytics", label: "roles.object.report_schedule" },
  report_edition: { area: "analytics", label: "roles.object.report_edition" },
  sales_target: { area: "analytics", label: "roles.object.sales_target" },
  reporting_framework: {
    area: "analytics",
    label: "roles.object.reporting_framework",
  },
  reporting_credit: {
    area: "analytics",
    label: "roles.object.reporting_credit",
  },

  capture_settings: { area: "data", label: "roles.object.capture_settings" },
  capture_trace: { area: "data", label: "roles.object.capture_trace" },
  channel_connection: {
    area: "data",
    label: "roles.object.channel_connection",
  },
  integrations: { area: "data", label: "roles.object.integrations" },
  webhook_subscription: {
    area: "data",
    label: "roles.object.webhook_subscription",
  },
  import_run: { area: "data", label: "roles.object.import_run" },
  finance: { area: "data", label: "roles.object.finance" },
  fx_rate: { area: "data", label: "roles.object.fx_rate" },
  data_coverage: { area: "data", label: "roles.object.data_coverage" },
  knowledge_corpus: { area: "data", label: "roles.object.knowledge_corpus" },
  knowledge_document: {
    area: "data",
    label: "roles.object.knowledge_document",
  },

  automation: { area: "ai", label: "roles.object.automation" },
  voice_profile: { area: "ai", label: "roles.object.voice_profile" },
  ai_routing: { area: "ai", label: "roles.object.ai_routing" },
  ai_budget: { area: "ai", label: "roles.object.ai_budget" },
  ai_diagnostics: { area: "ai", label: "roles.object.ai_diagnostics" },
  ai_model_rate: { area: "ai", label: "roles.object.ai_model_rate" },
  embedding_reindex: { area: "ai", label: "roles.object.embedding_reindex" },

  user_admin: { area: "people", label: "roles.object.user_admin" },
  role_admin: { area: "people", label: "roles.object.role_admin" },
  team_admin: { area: "people", label: "roles.object.team_admin" },
  team_lead: { area: "people", label: "roles.object.team_lead" },
  team_oversight: { area: "people", label: "roles.object.team_oversight" },
  seat_usage: { area: "people", label: "roles.object.seat_usage" },
  license: { area: "people", label: "roles.object.license" },

  installation_settings: {
    area: "company",
    label: "roles.object.installation_settings",
  },
  authentication_policy: {
    area: "company",
    label: "roles.object.authentication_policy",
  },
  oauth_application: {
    area: "company",
    label: "roles.object.oauth_application",
  },

  privacy_request: {
    area: "governance",
    label: "roles.object.privacy_request",
  },
  consent_config: { area: "governance", label: "roles.object.consent_config" },
  communication_exception: {
    area: "governance",
    label: "roles.object.communication_exception",
  },
  retention_policy: {
    area: "governance",
    label: "roles.object.retention_policy",
  },
  audit_log: { area: "governance", label: "roles.object.audit_log" },
  job_health: { area: "governance", label: "roles.object.job_health" },
  extension_access: {
    area: "governance",
    label: "roles.object.extension_access",
  },
  system_reset: { area: "governance", label: "roles.object.system_reset" },
};

/** The name a core object goes by, or undefined for one this client lacks. */
export function coreObjectName(object: string): CoreObjectName | undefined {
  return isCoreObject(object) ? CORE_OBJECTS[object] : undefined;
}

function isCoreObject(object: string): object is CoreRbacObject {
  return Object.hasOwn(CORE_OBJECTS, object);
}

// Not translated: the contract carries no label for an extension's object, so
// its key, less the unit's own prefix, is the only name there is.
export function objectLabel(unit: string, object: string): string {
  const prefix = `ext_${unit}_`;
  const bare = object.startsWith(prefix)
    ? object.slice(prefix.length)
    : object.replace(/^ext_/, "");
  const words = (bare || unit).replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}
