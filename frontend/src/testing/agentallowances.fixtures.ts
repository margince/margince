// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type RoleAllowance = components["schemas"]["AgentRoleAllowance"];
type Source = components["schemas"]["AgentRoleAllowanceValue"]["source"];

const inherited = (value: number) => ({
  value,
  source: "installation" as Source,
});

// A role row as the server answers it: each count its own value, the Admin
// preset, or the installation value it inherits.
export function roleAllowance(
  role_key: string,
  role_name: string,
  values: Partial<
    Record<
      | "agent_writes_per_window"
      | "agent_egress_per_window"
      | "agent_calls_per_window",
      { value: number; source: Source }
    >
  > = {},
  system = true,
): RoleAllowance {
  return {
    role_key,
    role_name,
    system,
    agent_writes_per_window: values.agent_writes_per_window ?? inherited(200),
    agent_egress_per_window: values.agent_egress_per_window ?? inherited(20),
    agent_calls_per_window: values.agent_calls_per_window ?? inherited(1_000),
  };
}

const adminAtPreset = roleAllowance("admin", "Admin", {
  agent_calls_per_window: { value: 20_000, source: "admin_preset" },
  agent_writes_per_window: { value: 2_000, source: "admin_preset" },
});

// The allowances an installation nobody has tuned reads back, for the stories
// and tests that answer the installation-settings read.
export const defaultAgentAllowances: components["schemas"]["AgentAllowanceSettings"] =
  {
    agent_writes_per_window: 200,
    agent_egress_per_window: 20,
    agent_calls_per_window: 1_000,
    agent_window_hours: 24,
    agent_window_resets_at: "2026-10-15T00:00:00Z",
    defaults: {
      agent_writes_per_window: 200,
      agent_egress_per_window: 20,
      agent_calls_per_window: 1_000,
      agent_window_hours: 24,
      admin_writes_per_window: 2_000,
      admin_calls_per_window: 20_000,
    },
    roles: [adminAtPreset, roleAllowance("rep", "User")],
    mcp_calls_per_minute: 240,
    mcp_stream_opens_per_minute: 30,
  };

// Two roles with values of their own beside the Admin preset, one of them made
// by an admin.
export const roleTunedAgentAllowances: components["schemas"]["AgentAllowanceSettings"] =
  {
    ...defaultAgentAllowances,
    roles: [
      adminAtPreset,
      roleAllowance("manager", "Team Lead", {
        agent_calls_per_window: { value: 3_000, source: "role" },
      }),
      roleAllowance("rep", "User"),
      roleAllowance("ops", "Ops / Integrations", {
        agent_calls_per_window: { value: 2_000, source: "role" },
        agent_writes_per_window: { value: 1_000, source: "role" },
      }),
      roleAllowance(
        "custom_field_research",
        "Field research",
        { agent_calls_per_window: { value: 5_000, source: "role" } },
        false,
      ),
    ],
  };

const counter = (used: number, limit: number) => ({ used, limit, released: 0 });

type From = components["schemas"]["AgentAllowanceFrom"];
const from = (limit: number, role_key: string | null = null): From => ({
  limit,
  role_key,
});

// Three users against their own limits: an Admin far inside the preset, a user
// whose two roles each raise one count and who is near a limit, and a User
// stopped at the installation value.
export const agentUsagePage: components["schemas"]["AgentUsageByUser"] = {
  resets_at: "2026-10-15T00:00:00Z",
  data: [
    {
      user_id: "u-anna",
      display_name: "Anna Weber",
      passport_count: 1,
      writes: counter(44, 2_000),
      egress: counter(2, 20),
      calls: counter(4_120, 20_000),
      state: "ok",
      roles: [{ role_key: "admin", role_name: "Admin" }],
      allowance: {
        writes: from(2_000, "admin"),
        egress: from(20),
        calls: from(20_000, "admin"),
      },
    },
    {
      user_id: "u-minh",
      display_name: "Minh Tran",
      passport_count: 1,
      writes: counter(612, 1_000),
      egress: counter(4, 20),
      calls: counter(2_840, 3_000),
      state: "near",
      roles: [
        { role_key: "manager", role_name: "Team Lead" },
        { role_key: "ops", role_name: "Ops / Integrations" },
      ],
      allowance: {
        writes: from(1_000, "ops"),
        egress: from(20),
        calls: from(3_000, "manager"),
      },
    },
    {
      user_id: "u-jonas",
      display_name: "Jonas Becker",
      passport_count: 1,
      writes: counter(80, 200),
      egress: counter(0, 20),
      calls: counter(1_000, 1_000),
      state: "stopped",
      roles: [{ role_key: "rep", role_name: "User" }],
      allowance: { writes: from(200), egress: from(20), calls: from(1_000) },
    },
  ],
};

// A passport mid-window: changes used up, calls well inside the ceiling.
export const busyUsage: components["schemas"]["AgentUsage"] = {
  writes: counter(200, 200),
  egress: counter(0, 20),
  calls: counter(640, 1_000),
  resets_at: "2026-10-15T00:00:00Z",
  state: "stopped",
};
