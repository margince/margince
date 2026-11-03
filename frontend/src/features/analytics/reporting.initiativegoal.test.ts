// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { bodyOf, goalOf, ruleOf } from "./reporting.initiativegoal";
import type { SalesInitiative } from "./reporting.initiatives";

// The initiative drawer's goal names who owns the initiative. An empty owner
// is left off the body, so the server makes the caller the owner.

const AT = "2026-10-01T09:00:00Z";
const saved: SalesInitiative = {
  id: "i-dach",
  name: "DACH expansion",
  owner_id: "u-maya",
  objective_metric: "bookings_won",
  deadline: "2026-12-31",
  status: "draft",
  include_future: true,
  version: 1,
  created_at: AT,
  updated_at: AT,
};

describe("the initiative goal's owner", () => {
  it("reads the owner a saved initiative names", () => {
    expect(goalOf(saved).ownerId).toBe("u-maya");
  });

  it("sends the owner the drawer names", () => {
    const goal = { ...goalOf(saved), ownerId: "u-sam" };
    expect(bodyOf(goal, ruleOf(saved)).owner_id).toBe("u-sam");
  });

  it("leaves the owner off a new initiative nobody named one for", () => {
    const body = bodyOf(
      { ...goalOf(), name: "New push", deadline: "2026-12-31" },
      ruleOf(),
    );
    expect(body).not.toHaveProperty("owner_id");
  });
});
