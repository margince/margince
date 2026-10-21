/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { view } from "../../testing/contactpage.testkit";
import { recipientSuggestions } from "./composehead";

type User = components["schemas"]["User"];

function member(overrides: Partial<User>): User {
  return {
    id: "u-1",
    email: "sofia@gradion.test",
    display_name: "Sofia Keller",
    timezone: UTC_ZONE,
    status: "active",
    is_agent: false,
    ...overrides,
  };
}

describe("recipientSuggestions", () => {
  it("offers workspace members after the record's own contacts", () => {
    const offered = recipientSuggestions(view, undefined, [member({})]);
    expect(offered.map((row) => row.value)).toEqual([
      "dana@brandt.example",
      "sofia@gradion.test",
    ]);
    expect(offered[1]?.label).toBe("Sofia Keller");
  });

  it("leaves out agents and members who cannot read mail", () => {
    const offered = recipientSuggestions(undefined, undefined, [
      member({ id: "a", email: "bot@gradion.test", is_agent: true }),
      member({ id: "s", email: "gone@gradion.test", status: "suspended" }),
      member({ id: "d", email: "left@gradion.test", status: "deactivated" }),
      member({ id: "i", email: "new@gradion.test", status: "invited" }),
    ]);
    expect(offered.map((row) => row.value)).toEqual(["new@gradion.test"]);
  });

  it("skips teams, which have no address", () => {
    const offered = recipientSuggestions(undefined, undefined, [
      { id: "t-1", name: "Sales" },
      member({}),
    ]);
    expect(offered.map((row) => row.value)).toEqual(["sofia@gradion.test"]);
  });

  it("offers an address once when a contact and a member share it", () => {
    const offered = recipientSuggestions(undefined, undefined, [
      member({}),
      member({ id: "u-2", display_name: "Sofia K." }),
    ]);
    expect(offered).toHaveLength(1);
  });
});
