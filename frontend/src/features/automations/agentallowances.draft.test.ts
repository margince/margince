// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  defaultAgentAllowances,
  roleTunedAgentAllowances,
} from "../../testing/agentallowances.fixtures";
import {
  adminPresetDraft,
  defaultsDraft,
  EMPTY_ROLE_DRAFT,
  parseRoleDraft,
  roleChangesOf,
  roleDraftsOf,
} from "./agentallowances.draft";

describe("roleChangesOf", () => {
  const drafts = roleDraftsOf(roleTunedAgentAllowances.roles);

  it("sends nothing while no role moved, so the preset stays the preset", () => {
    expect(roleChangesOf(roleTunedAgentAllowances, drafts)).toBeUndefined();
  });

  it("sends every role with a value once one moved, Admin at the preset left out", () => {
    const sent = roleChangesOf(roleTunedAgentAllowances, {
      ...drafts,
      rep: { ...EMPTY_ROLE_DRAFT, agent_egress_per_window: "40" },
    });
    expect(sent?.map((set) => set.role_key)).toEqual([
      "manager",
      "rep",
      "ops",
      "custom_field_research",
    ]);
  });

  it("sends Admin typed off the preset with what was typed", () => {
    const sent = roleChangesOf(defaultAgentAllowances, {
      admin: {
        ...adminPresetDraft(defaultAgentAllowances),
        agent_calls_per_window: "9000",
      },
    });
    expect(sent).toEqual([
      {
        role_key: "admin",
        agent_calls_per_window: 9000,
        agent_writes_per_window: 2000,
        agent_egress_per_window: 0,
      },
    ]);
  });
});

describe("parseRoleDraft", () => {
  it("reads an empty box as the installation value and refuses one out of range", () => {
    expect(
      parseRoleDraft({
        agent_calls_per_window: "",
        agent_writes_per_window: "9",
        agent_egress_per_window: "200",
      }),
    ).toEqual({
      agent_calls_per_window: 0,
      agent_writes_per_window: null,
      agent_egress_per_window: 200,
    });
  });
});

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
