// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { AgentClient } from "../features/automations/agent-clients.queries";

type ListResponse = components["schemas"]["AgentClientListResponse"];
type Registration = components["schemas"]["AgentClientRegistration"];

export const BOTH_METHODS_ON: Registration = {
  dynamic: { enabled: true, setting: true, deployment_allows: true },
  metadata_document: { enabled: true, setting: true, deployment_allows: true },
};

export const GEMINI_CLIENT: AgentClient = {
  id: "c0000000-0000-4000-8000-000000000001",
  client_id: "mc_7f3a9d2c41e8",
  client_name: "Gemini Enterprise",
  redirect_uris: ["https://vertexaisearch.cloud.google.com/oauth-redirect"],
  registered_by: "admin",
  confidential: true,
  disabled: false,
  created_at: "2026-09-01T08:00:00Z",
  connected_users: 12,
  last_used_at: "2026-10-18T09:30:00Z",
};

export const SELF_REGISTERED_CLIENT: AgentClient = {
  id: "c0000000-0000-4000-8000-000000000002",
  client_id: "dcr_51b0e7a2",
  client_name: "Old desktop tool",
  redirect_uris: ["http://127.0.0.1/callback"],
  registered_by: "dcr",
  confidential: false,
  disabled: true,
  created_at: "2026-05-01T08:00:00Z",
  connected_users: 0,
  last_used_at: null,
};

export const PUBLISHED_CLIENT: AgentClient = {
  id: "c0000000-0000-4000-8000-000000000003",
  client_id: "https://claude.ai/oauth/claude-code-client-metadata",
  client_name: "Claude Code",
  redirect_uris: ["http://localhost/callback"],
  registered_by: "cimd",
  confidential: false,
  disabled: false,
  created_at: "2026-08-12T08:00:00Z",
  connected_users: 1,
  last_used_at: "2026-10-17T15:00:00Z",
};

export function agentClientList(
  data: readonly AgentClient[],
  registration: Registration = BOTH_METHODS_ON,
  nextCursor: string | null = null,
): ListResponse {
  return {
    data: [...data],
    page: { next_cursor: nextCursor, has_more: nextCursor !== null },
    registration,
  };
}

// The scope list the server sends with a new or rotated secret.
export const AGENT_CLIENT_SCOPES =
  "read draft write send enrich offline_access";
