// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  defaultAgentAllowances,
  roleAllowance,
  roleTunedAgentAllowances,
} from "../../testing/agentallowances.fixtures";
import {
  defaultsDraft,
  EMPTY_ROLE_DRAFT,
  parseRoleCell,
  roleChangesOf,
  roleDraftsOf,
} from "./agentallowances.draft";

describe("roleChangesOf", () => {
  const drafts = roleDraftsOf(roleTunedAgentAllowances.roles);

  it("sends nothing while no box moved", () => {
    expect(roleChangesOf(roleTunedAgentAllowances, drafts)).toBeUndefined();
  });

  it("sends only the role and the count that moved", () => {
    const sent = roleChangesOf(roleTunedAgentAllowances, {
      ...drafts,
      rep: { ...EMPTY_ROLE_DRAFT, agent_egress_per_window: "40" },
    });
    expect(sent).toEqual([{ role_key: "rep", agent_egress_per_window: 40 }]);
  });

  it("gives a value up when its box is emptied", () => {
    const sent = roleChangesOf(roleTunedAgentAllowances, {
      ...drafts,
      manager: EMPTY_ROLE_DRAFT,
    });
    expect(sent).toEqual([
      { role_key: "manager", unset: ["agent_calls_per_window"] },
    ]);
  });

  // The preset binds changes while an operator chose calls. Neither value is
  // one the admin saved for Admin, so editing another role sends no Admin.
  it("leaves a partly applied preset out of a save of another role", () => {
    const partial = {
      ...defaultAgentAllowances,
      agent_calls_per_window: 5_000,
      roles: [
        roleAllowance(
          "admin",
          "Admin",
          {
            agent_calls_per_window: { value: 5_000, source: "installation" },
            agent_writes_per_window: { value: 2_000, source: "admin_preset" },
          },
          true,
          true,
        ),
        roleAllowance("rep", "User"),
      ],
    };
    const sent = roleChangesOf(partial, {
      ...roleDraftsOf(partial.roles),
      rep: { ...EMPTY_ROLE_DRAFT, agent_calls_per_window: "3000" },
    });
    expect(sent).toEqual([{ role_key: "rep", agent_calls_per_window: 3000 }]);
  });
});

describe("a kept installation value", () => {
  const admin = roleAllowance(
    "admin",
    "Admin",
    { agent_calls_per_window: { value: 1_000, source: "installation_kept" } },
    true,
    true,
  );
  const allowances = { ...defaultAgentAllowances, roles: [admin] };

  it("starts as 0 and, emptied, is given up so Admin takes its preset again", () => {
    const drafts = roleDraftsOf(allowances.roles);
    expect(drafts.admin.agent_calls_per_window).toBe("0");
    expect(roleChangesOf(allowances, { admin: EMPTY_ROLE_DRAFT })).toEqual([
      { role_key: "admin", unset: ["agent_calls_per_window"] },
    ]);
  });
});

describe("parseRoleCell", () => {
  it("reads empty as no value, 0 as the installation value, and refuses one out of range", () => {
    expect(parseRoleCell("agent_calls_per_window", "")).toBeUndefined();
    expect(parseRoleCell("agent_calls_per_window", "0")).toBe(0);
    expect(parseRoleCell("agent_writes_per_window", "9")).toBeNull();
    expect(parseRoleCell("agent_egress_per_window", "200")).toBe(200);
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
