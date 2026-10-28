// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";
import {
  agentClientList,
  GEMINI_CLIENT,
  PUBLISHED_CLIENT,
} from "../../src/testing/agent-clients.fixtures";
import { defaultAgentAllowances } from "../../src/testing/agentallowances.fixtures";
import { meFixture } from "../../src/testing/mefixture";
import { defaultOperations } from "../../src/testing/operationsettings.fixtures";
import { E2E_ADMIN_GRANTS } from "./fixtures";
import { type Handler, notFound } from "./server";

// The signed-in principal, and the 401 a signed-out run gets. The 401 has a
// problem body and is never an abort. An abort lands the caller on the
// connection screen, which is for a server that is down.
export const me: Handler = ({ json }, { options }) => {
  if (options.session === "unauthenticated") {
    return json(
      { type: "about:blank", title: "Unauthorized", status: 401 },
      401,
    );
  }
  const fixture = meFixture({
    allow:
      options.dataTasks === "withheld"
        ? { ...E2E_ADMIN_GRANTS, data_task: [] }
        : E2E_ADMIN_GRANTS,
  });
  return json({
    ...fixture,
    // Armed, so the sweep opens `settings/reset`. Rendering the danger zone
    // resets nothing: no spec types the workspace name and presses the button.
    data_reset_available: true,
    user: {
      ...fixture.user,
      id: "u1",
      email: "lars@brandt.example",
      // "de", not "de-DE": the contract declares en | de | vi, and the
      // catalogs are keyed by those three.
      locale: "de",
    },
  });
};

// When this reader is bookable. A PUT stores the hours as chosen and answers
// with them, as the server does, so a saved card stops offering its default.
export const myWorkingHours: Handler = ({ route, json }, state) => {
  if (route.request().method() === "PUT") {
    state.workingHours = {
      chosen: true,
      working_hours: route.request().postDataJSON(),
    };
  }
  return json(state.workingHours);
};

// All six classes, because the contract promises the whole set and the
// screen draws one row per entry. Two follow the installation's default, so
// the axe passes see both renderings of a row.
export const notificationPreferences = {
  items: [
    { class: "approval_pending", delivery: "email", chosen: true },
    { class: "lead_sla", delivery: "digest", chosen: true },
    { class: "automation", delivery: "in_app", chosen: false },
    { class: "capture", delivery: "off", chosen: true },
    { class: "coach", delivery: "in_app", chosen: false },
    { class: "system", delivery: "in_app", chosen: true },
  ],
};

function redirectUris(microsoft: boolean) {
  const host = "https://api.brandt.example/v1";
  const [signIn, mail, calendar] = microsoft
    ? ["oidc/microsoft", "connectors/graph", "connectors/graphcal"]
    : ["oidc/google", "connectors/gmail", "connectors/gcal"];
  return [
    { purpose: "sign_in", url: `${host}/auth/${signIn}/callback` },
    { purpose: "mailbox_connect", url: `${host}/${mail}/callback` },
    { purpose: "calendar_connect", url: `${host}/${calendar}/callback` },
  ];
}

// One card per vendor, each supplied by the deployment (`environment`), the
// state the card was written for. Only Microsoft has directories, and this
// app is pinned to one.
function coreOAuthApp(microsoft: boolean) {
  return {
    provider: microsoft ? "microsoft" : "google",
    configured: true,
    client_id: microsoft
      ? "11111111-2222-3333-4444-555555555555"
      : "000000000000-brandt.apps.googleusercontent.com",
    ...(microsoft ? { tenant: "99999999-8888-7777-6666-555555555555" } : {}),
    source: "environment",
    redirect_uris: redirectUris(microsoft),
  };
}

export const oauthApp: Handler = ({ json, path }) =>
  json(coreOAuthApp(path.endsWith("/microsoft")));

// The vanilla tree composes no unit, so the list is the two core vendors.
export const oauthApps: Handler = ({ json }) =>
  json({ data: [coreOAuthApp(false), coreOAuthApp(true)] });

// The shell holds its first paint on this read: `timezone` is the clock every
// record date renders in. Europe/Berlin, because the specs assert German
// dates against it.
// The MCP page's client list: one client an admin registered and one that
// signs in with its published identity.
export const agentClients = agentClientList([GEMINI_CLIENT, PUBLISHED_CLIENT]);

export const installationSettings = {
  name: "Brandt Automotive",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
  base_language: "de",
  base_currency_locked: false,
  max_upload_bytes: 25_000_000,
  oauth_access_token_ttl_minutes: 43_200,
  operations: defaultOperations,
  agent_allowances: defaultAgentAllowances,
  // One provider in each state, so the sign-in methods card draws both rows.
  sign_in_providers: [
    { key: "google", label: "Google", enabled: true },
    { key: "microsoft", label: "Microsoft", enabled: false },
  ],
};

// The configuration list: a value from each kind of source, a secret, and a
// struct value long enough to need wrapping at 390px.
export const installationConfiguration: components["schemas"]["ConfigurationEntry"][] =
  [
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
      origin: { source: "flag", name: "--addr" },
    },
    {
      key: "database.dsn",
      env: "MARGINCE_DSN",
      type: "string",
      doc: "The connection string of the database.",
      roles: ["api", "worker"],
      secret: true,
      stored: false,
      value: null,
      default: null,
      origin: { source: "env", name: "MARGINCE_DSN" },
    },
    {
      key: "ai.budget",
      env: "MARGINCE_AI_BUDGET",
      type: "struct",
      doc: "The AI token allowance per seat and per company.",
      roles: ["api", "worker"],
      secret: false,
      stored: true,
      value:
        '{"tokens_per_full_user":12000000,"company_monthly_tokens":null,"tokens_per_light_user":3000000}',
      default: null,
      origin: { source: "database", name: "" },
    },
    {
      key: "installation.follow_up_reconcile_interval_seconds",
      env: "MARGINCE_INSTALLATION_FOLLOW_UP_RECONCILE_INTERVAL_SECONDS",
      type: "int",
      doc: "How often the follow-up reconcile pass runs.",
      roles: ["worker"],
      secret: false,
      stored: true,
      value: "900",
      default: "900",
      origin: { source: "default", name: "" },
    },
  ];

// The banner keys on the two identities differing and on nothing else. It
// does not read `reindex_needed`, which a drift-cancelled rebuild leaves set.
// The two identities, shared by the shell banner's read and the status read
// System health makes, so the two answer one installation.
function reindexIdentities(needed: boolean) {
  return {
    configured_identity: "bge-m3@1024",
    populated_identity: needed ? "nomic-embed-text@768" : "bge-m3@1024",
  };
}

export const reindexStatus: Handler = ({ json }, { options }) => {
  const needed = options.embedReindex === "needed";
  return json({
    ...reindexIdentities(needed),
    reindex_needed: needed,
    status: "idle",
  });
};

export const reindexIdentitiesRead: Handler = ({ json }, { options }) =>
  json(reindexIdentities(options.embedReindex === "needed"));

// No OIDC provider unless a test seeds one: the flow has not shipped, and the
// empty list proves no provider button renders.
export const authCapabilities: Handler = ({ json }, { options }) =>
  json({
    password: true,
    password_reset: true,
    oidc_providers: options.oidcProviders ?? [],
  });

export const assistantProfile = {
  name: "Margince",
  kind: "ai",
  state: "configured",
  inference_mode: "hybrid",
  providers: ["anthropic", "ollama"],
};

export const companyContextCapabilities = {
  rollout: "onboarding",
  read_enabled: true,
  tasks_enabled: true,
  onboarding_enabled: true,
};

// What the installation still needs before it can think. Only "unconfigured"
// leaves the model step open, which puts it in front of the website step.
export const installationSetup: Handler = ({ json }, { options }) => {
  const configured = options.journey !== "unconfigured";
  return json({
    complete: configured,
    steps: [
      { step: "ai_models", configured, blocking: true },
      { step: "oauth_app", configured, blocking: false },
    ],
  });
};

// The acting human's wizard row. The shell gates on it, so a described
// installation answers a finished creator row. An undescribed one has no row,
// which the contract spells as 404.
export const onboardingState: Handler = ({ json }, { undescribed }) => {
  if (undescribed) {
    return json(notFound, 404);
  }
  return json({
    path: "creator",
    step: "complete",
    source_mode: "website",
    website_url: "https://brandt.example",
    company_draft: { display_name: "Brandt Automotive GmbH" },
    selected_fact_keys: [],
    voice_skipped: false,
    connect_skipped: false,
    version: 9,
    completed_at: "2026-06-01T09:00:00Z",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T09:00:00Z",
  });
};

// The installation's own company, shaped as the contract's CompanyProfile.
// The shell gates on it as well.
export const company = {
  company_id: "o-self",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  registered_address: "Werkstraße 4, 70435 Stuttgart",
  register_vat: "DE811234567",
  industry: "Automotive",
  website: "brandt.example",
};
