// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { recipientSuggestions } from "./composehead";

type User = components["schemas"]["User"];

function member(overrides: Partial<User>): User {
  return {
    id: "u-1",
    email: "sofia@gradion.test",
    display_name: "Sofia Keller",
    status: "active",
    is_agent: false,
    ...overrides,
  };
}

describe("recipientSuggestions", () => {
  it("offers workspace members after the record's own contacts", () => {
    const offered = recipientSuggestions(
      {
        as_of: "2026-08-15T09:00:00Z",
        contact: {
          id: "p-1",
          full_name: "Dana Brandt",
          emails: [{ email: "dana@brandt.example", is_primary: true }],
        },
        sections_omitted: [],
      } as components["schemas"]["Contact360"],
      undefined,
      [member({})],
    );
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

  it("offers an address once when a contact and a member share it", () => {
    const offered = recipientSuggestions(undefined, undefined, [
      member({}),
      member({ id: "u-2", display_name: "Sofia K." }),
    ]);
    expect(offered).toHaveLength(1);
  });
});
