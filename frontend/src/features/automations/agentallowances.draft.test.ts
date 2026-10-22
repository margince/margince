// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { defaultsDraft } from "./agentallowances.draft";

const TUNED = {
  ...defaultAgentAllowances,
  agent_writes_per_window: 500,
  agent_calls_per_window: 4_000,
  agent_window_hours: 6,
};

describe("defaultsDraft", () => {
  it("keeps a locked value in force and takes the default for the rest", () => {
    const draft = defaultsDraft(
      TUNED,
      (property) => property === "agent_writes_per_window",
    );
    expect(draft).toEqual({
      agent_writes_per_window: "500",
      agent_egress_per_window: "20",
      agent_calls_per_window: "1000",
      agent_window_hours: "24",
    });
  });

  it("takes every default when nothing is locked", () => {
    expect(defaultsDraft(TUNED, () => false)).toEqual({
      agent_writes_per_window: "200",
      agent_egress_per_window: "20",
      agent_calls_per_window: "1000",
      agent_window_hours: "24",
    });
  });
});
