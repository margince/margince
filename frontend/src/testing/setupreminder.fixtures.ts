// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reads behind the top bar's "Finish setup" reminder, shared by the stories
// that draw it. The default world: Gmail connected, the calendar skipped, so
// booking waits on it.

import type { GrantSpec } from "./mefixture";
import { jsonResponse, type RouteMap } from "./story-utils";

export const gmail = {
  id: "00000000-0000-4000-8000-000000000011",
  provider: "gmail",
  status: "connected",
  scopes: [],
  account_label: "ada@example.test",
};

export type SetupWorld = Readonly<{
  rows?: readonly object[];
  reason?: string;
  finished?: boolean;
  aiConnected?: boolean;
  /** An admin who skipped the AI key on first run: no model is bound. */
  modelUnbound?: boolean;
}>;

// An AI tool the reader approved through sign-in, which finishes the AI step.
const claudeConnection = {
  id: "00000000-0000-4000-8000-000000000021",
  label: "Claude",
  scopes: ["read", "draft"],
  created_at: "2026-11-01T08:00:00Z",
  last_used_at: "2026-11-04T10:00:00Z",
  expires_at: null,
  revoked_at: null,
  connection: {
    client_id: "claude-ai",
    client_name: "Claude",
    renewable: true,
    connected_at: "2026-11-01T08:00:00Z",
  },
};

export function setupReminderRoutes(world: SetupWorld = {}): RouteMap {
  return {
    "GET /connectors": () =>
      jsonResponse({
        data: world.rows ?? [gmail],
        providers: ["gmail", "graph", "imap"].map((provider) => ({
          provider,
          reason: world.reason ?? "ready",
        })),
      }),
    "GET /voice-profiles": () =>
      jsonResponse({
        data: world.finished ? [{ profile_version: 2 }] : [],
      }),
    "GET /scheduling/profile": () =>
      jsonResponse({
        enabled: world.finished ?? false,
        provider: "",
        calendar_id: "",
        duration_minutes: 30,
        notice_minutes: 240,
        horizon_days: 30,
        buffer_minutes: 0,
        title: "Intro call",
        location: "",
      }),
    "GET /.well-known/oauth-protected-resource": () =>
      jsonResponse({ resource: "https://margince.example/mcp" }),
    "GET /installation/setup": () =>
      jsonResponse({
        complete: !world.modelUnbound,
        steps: [
          {
            step: "ai_models",
            configured: !world.modelUnbound,
            blocking: true,
          },
          { step: "oauth_app", configured: true, blocking: false },
        ],
      }),
    "GET /passports": () =>
      jsonResponse({
        data: (world.aiConnected ?? world.finished) ? [claudeConnection] : [],
        api_base_url: "https://margince.example/v1",
        page: { next_cursor: null, has_more: false },
      }),
  };
}

/** The grants the reminder's steps read, for `stubWithSession`. */
export function setupReminderGrants(world: SetupWorld = {}): GrantSpec {
  return {
    voice_profile: ["read", "create", "update"],
    ...(world.modelUnbound ? { ai_routing: ["read", "update"] } : {}),
  };
}
