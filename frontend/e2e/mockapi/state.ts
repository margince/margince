// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";
import { projectMock } from "../projectmock";
import {
  brandt,
  briefRun,
  deals,
  seededAutomation,
  seededProject,
  seededViews,
} from "./fixtures";

export type MockApiOptions = Readonly<{
  // "authenticated" (the default) is what every existing AC needs.
  // "unauthenticated" answers /me with 401, so the app renders the login
  // screen a signed-out user meets. The axe sweeps cannot reach it otherwise.
  session?: "authenticated" | "unauthenticated";
  // The federated providers /auth/capabilities reports. The default is `[]`
  // because the OIDC flow has not shipped, and the empty list proves no
  // provider button renders. A test that wants the federated block seeds it
  // here, the one place in this repo where a provider exists.
  oidcProviders?: ReadonlyArray<{ key: string; label: string }>;
  // Which meeting brief the drawer gets. The default is the everyday one,
  // carrying a withheld source for the AC that reads it.
  meetingBrief?: "rep" | "plan" | "manager" | "empty" | "failed";
  // Where the acting human stands in the setup journey. "finished" (the
  // default) is described and complete. The shell lets every screen render.
  // "unstarted" has no company and no wizard row, which the onboarding AC
  // needs. "unconfigured" is "unstarted" with no model bound either. The cold
  // start then asks for the model first, and its long form is where the
  // way-onward-stays-in-view claim is measured.
  journey?: "finished" | "unstarted" | "unconfigured";
  // What the write gate answers for the seeded project. "read-only" is the
  // state a record page draws its read-only band for.
  project?: "writable" | "read-only";
  // Whether the embedding index matches the bound model. "needed" is the
  // mismatch that raises the shell's standing advisory.
  embedReindex?: "current" | "needed";
  // What the licence resolved to. "valid" (the default) is the seat-count
  // world of the Seats page, "absent" is every dev and demo stack, and
  // "rejected" raises the shell's licence banner.
  license?: "valid" | "absent" | "rejected";
  // One run in flight on the agent feed. Without it the rail is at rest in
  // every spec and its live line is never drawn.
  agentRunning?: true;
}>;

export type LoggedActivity = {
  id: string;
  workspace_id: string;
  kind: string;
  subject: string;
  body: string | null;
  occurred_at: string;
  thread_key: null;
  links: { entity_type: string; entity_id: string }[];
  source: string;
  captured_by: string;
  version: number;
  created_at: string;
  updated_at: string;
};

export type CaptureSender = {
  address: string;
  kind?: string;
  status?: string;
  decision?: string;
  overruled_kind?: string;
  overruled: boolean;
  record_exists: boolean;
};

export type CounterpartyHold = {
  id: string;
  kind: string;
  value: string;
  created_at: string;
};

export type DealPatches = Record<string, Partial<(typeof deals)[number]>>;

function captureSettings(): components["schemas"]["CaptureSettings"] {
  return {
    auto_enrich: true,
    mail_sharing: true,
    shared_posture_allowed: false,
    signature_enrich: true,
    auto_enrich_daily_cap: 500,
    site_read: { max_pages: 60, max_mib: 32, wall_seconds: 240 },
    mail_sync_interval_seconds: 120,
  };
}

function captureConnections() {
  return [
    {
      id: "conn-gmail",
      provider: "gmail",
      status: "connected",
      account_label: "admin@demo.test",
      scopes: [],
      mail_posture: "classified",
      backfill: { state: "none" },
    },
  ];
}

// One of each row the Senders table draws differently: an admitted kind, a
// refused one, and a personal sender.
function captureSenders(): CaptureSender[] {
  return [
    {
      address: "jana@commercetools.com",
      kind: "contact",
      status: "real",
      overruled: false,
      record_exists: true,
    },
    {
      address: "news@substack.com",
      kind: "newsletter",
      status: "noise",
      overruled: false,
      record_exists: false,
    },
    {
      address: "anne@hotmail.com",
      kind: "personal",
      status: "noise",
      overruled: false,
      record_exists: false,
    },
  ];
}

// Everything a page's writes change, created fresh for each mockApi call.
// A write is read back within one test, and never leaks into the next.
export function createMockState(options: MockApiOptions = {}) {
  const dealPatches: DealPatches = {};
  // Only activities linked to a deal or a project are kept, so the timelines
  // the other suites read as empty stay empty.
  const loggedActivities: LoggedActivity[] = [];
  const counterpartyHolds: CounterpartyHold[] = [];
  return {
    options,
    // No company row and no wizard row. Named once, so a third journey state
    // cannot be added to one route and missed by the other.
    undescribed:
      options.journey === "unstarted" || options.journey === "unconfigured",
    automations: [{ ...seededAutomation }],
    dealPatches,
    views: seededViews.map((view) => ({ ...view })),
    brief: {
      ...briefRun,
      items: briefRun.items.map((item) => ({ ...item })),
    },
    captureSettings: captureSettings(),
    captureConnections: captureConnections(),
    captureSenders: captureSenders(),
    counterpartyHolds,
    loggedActivities,
    projectState: projectMock({
      seeded: { ...seededProject, writable: options.project !== "read-only" },
      companyName: brandt.display_name,
      deals: () => deals.map((deal) => ({ ...deal, ...dealPatches[deal.id] })),
      activities: () => loggedActivities,
    }),
  };
}

export type MockState = ReturnType<typeof createMockState>;
