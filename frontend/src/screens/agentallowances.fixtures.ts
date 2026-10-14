// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

// The allowances an installation nobody has tuned reads back, for the stories
// and tests that answer the installation-settings read.
export const defaultAgentAllowances: components["schemas"]["AgentAllowanceSettings"] =
  {
    agent_reads_per_window: 2_000,
    agent_writes_per_window: 200,
    agent_egress_per_window: 20,
    agent_calls_per_window: 1_000,
    agent_window_hours: 24,
    agent_window_resets_at: "2026-10-15T00:00:00Z",
    mcp_calls_per_minute: 240,
    mcp_stream_opens_per_minute: 30,
  };

const counter = (used: number, limit: number) => ({ used, limit, released: 0 });

// A passport mid-window: reads close to their limit, changes used up.
export const busyUsage: components["schemas"]["AgentUsage"] = {
  reads: counter(1_840, 2_000),
  writes: counter(200, 200),
  egress: counter(0, 20),
  calls: counter(640, 1_000),
  resets_at: "2026-10-15T00:00:00Z",
  state: "stopped",
};
